package nvd

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/alt-cloud/trivy-db/pkg/db"
	"github.com/alt-cloud/trivy-db/pkg/types"
	"github.com/alt-cloud/trivy-db/pkg/utils"
	"github.com/alt-cloud/trivy-db/pkg/vulnsrc/vulnerability"
	"github.com/samber/lo"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

const (
	vulnListDir = "vuln-list-nvd"
	apiDir      = "api"
	nvdSource   = "nvd@nist.gov"
)

type VulnSrc struct {
	dbc db.Operation
}

func NewVulnSrc() VulnSrc {
	return VulnSrc{
		dbc: db.Config{},
	}
}

func (vs VulnSrc) Name() types.SourceID {
	return vulnerability.NVD
}

func (vs VulnSrc) Update(dir string) error {
	rootDir := filepath.Join(dir, vulnListDir, apiDir)

	var cves []Cve
	buffer := &bytes.Buffer{}
	err := utils.FileWalk(rootDir, func(r io.Reader, _ string) error {
		cve := Cve{}
		if _, err := buffer.ReadFrom(r); err != nil {
			return xerrors.Errorf("failed to read file: %w", err)
		}
		if err := json.Unmarshal(buffer.Bytes(), &cve); err != nil {
			return xerrors.Errorf("failed to decode NVD JSON: %w", err)
		}
		buffer.Reset()
		cves = append(cves, cve)
		return nil
	})
	if err != nil {
		return xerrors.Errorf("error in NVD walk: %w", err)
	}

	if err = vs.save(cves); err != nil {
		return xerrors.Errorf("error in NVD save: %w", err)
	}

	return nil
}

func (vs VulnSrc) commit(tx *bolt.Tx, cves []Cve) error {
	for _, cve := range cves {
		cveID := cve.ID

		cvssScore, cvssVector, severity := getCvssV2(cve.Metrics.CvssMetricV2)
		cvssScoreV3, cvssVectorV3, severityV3 := getCvssV3(cve.Metrics.CvssMetricV31, cve.Metrics.CvssMetricV30)

		var references []string
		for _, ref := range cve.References {
			references = append(references, ref.URL)
		}

		var description string
		for _, d := range cve.Descriptions {
			if d.Value != "" {
				description = d.Value
				break
			}
		}
		var cweIDs []string
		for _, data := range cve.Weaknesses {
			for _, desc := range data.Description {
				if !strings.HasPrefix(desc.Value, "CWE") {
					continue
				}
				cweIDs = append(cweIDs, desc.Value)
			}
		}

		publishedDate, _ := time.Parse("2006-01-02T15:04:05", cve.Published)
		lastModifiedDate, _ := time.Parse("2006-01-02T15:04:05", cve.LastModified)

		vuln := types.VulnerabilityDetail{
			CvssScore:        cvssScore,
			CvssVector:       cvssVector,
			CvssScoreV3:      cvssScoreV3,
			CvssVectorV3:     cvssVectorV3,
			Severity:         severity,
			SeverityV3:       severityV3,
			CweIDs:           lo.Uniq(cweIDs),
			References:       references,
			Title:            "",
			Description:      description,
			PublishedDate:    &publishedDate,
			LastModifiedDate: &lastModifiedDate,
		}

		if err := vs.dbc.PutVulnerabilityDetail(tx, cveID, vulnerability.NVD, vuln); err != nil {
			return err
		}
	}
	return nil
}

func (vs VulnSrc) save(cves []Cve) error {
	log.Println("NVD batch update")
	err := vs.dbc.BatchUpdate(func(tx *bolt.Tx) error {
		return vs.commit(tx, cves)
	})
	if err != nil {
		return xerrors.Errorf("error in batch update: %w", err)
	}
	return nil
}

// getCvssV2 selects vector, score and severity from V2 metrics
func getCvssV2(metricsV2 []CvssMetricV2) (score float64, vector string, severity types.Severity) {
	for _, metricV2 := range metricsV2 {
		// save only NVD metric
		if metricV2.Source == nvdSource {
			score = metricV2.CvssData.BaseScore
			vector = metricV2.CvssData.VectorString
			severity, _ = types.NewSeverity(metricV2.BaseSeverity)
			return
		}
	}
	return
}

// getCvssV3 selects vector, score and severity from V3* metrics
func getCvssV3(metricsV31, metricsV30 []CvssMetricV3) (score float64, vector string, severity types.Severity) {
	// order: v3.1 metrics => v3.0 metrics
	// save the first NVD metric
	for _, metricV3 := range append(metricsV31, metricsV30...) {
		if metricV3.Source == nvdSource {
			score = metricV3.CvssData.BaseScore
			vector = metricV3.CvssData.VectorString
			severity, _ = types.NewSeverity(metricV3.CvssData.BaseSeverity)
			return
		}
	}
	return
}
