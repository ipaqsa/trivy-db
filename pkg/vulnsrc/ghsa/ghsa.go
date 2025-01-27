package ghsa

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alt-cloud/trivy-db/pkg/types"
	"github.com/alt-cloud/trivy-db/pkg/vulnsrc/osv"
	"github.com/alt-cloud/trivy-db/pkg/vulnsrc/vulnerability"
	"golang.org/x/xerrors"
)

const (
	sourceID       = vulnerability.GHSA
	platformFormat = "GitHub Security Advisory %s"
	urlFormat      = "https://github.com/advisories?query=type%%3Areviewed+ecosystem%%3A%s"
)

var (
	ghsaDir = filepath.Join("ghsa", "advisories", "github-reviewed")

	// Mapping between Trivy ecosystem and GHSA ecosystem
	ecosystems = map[types.Ecosystem]string{
		vulnerability.Composer:  "Composer",
		vulnerability.Go:        "Go",
		vulnerability.Maven:     "Maven",
		vulnerability.Npm:       "npm",
		vulnerability.NuGet:     "NuGet",
		vulnerability.Pip:       "pip",
		vulnerability.RubyGems:  "RubyGems",
		vulnerability.Cargo:     "Rust", // different name
		vulnerability.Erlang:    "Erlang",
		vulnerability.Pub:       "Pub",
		vulnerability.Swift:     "Swift",
		vulnerability.Cocoapods: "Swift", // Use Swift advisories for CocoaPods
	}
)

type DatabaseSpecific struct {
	Severity string `json:"severity"`
}

type GHSA struct{}

func NewVulnSrc() GHSA {
	return GHSA{}
}

func (GHSA) Name() types.SourceID {
	return vulnerability.GHSA
}

func (GHSA) Update(root string) error {
	dataSources := map[types.Ecosystem]types.DataSource{}
	for ecosystem, ghsaEcosystem := range ecosystems {
		src := types.DataSource{
			ID:   sourceID,
			Name: fmt.Sprintf(platformFormat, ghsaEcosystem),
			URL:  fmt.Sprintf(urlFormat, strings.ToLower(ghsaEcosystem)),
		}
		dataSources[ecosystem] = src
	}

	t, err := newTransformer(root)
	if err != nil {
		return xerrors.Errorf("transformer error: %w", err)
	}

	return osv.New(ghsaDir, sourceID, dataSources, t).Update(root)
}

type transformer struct {
	// cocoaPodsSpecs is a map of Swift git URLs to CocoaPods package names.
	cocoaPodsSpecs map[string][]string
}

func newTransformer(root string) (*transformer, error) {
	cocoaPodsSpecs, err := walkCocoaPodsSpecs(root)
	if err != nil {
		return nil, xerrors.Errorf("CocoaPods spec error: %w", err)
	}
	return &transformer{
		cocoaPodsSpecs: cocoaPodsSpecs,
	}, nil
}

func (t *transformer) TransformAdvisories(advisories []osv.Advisory, entry osv.Entry) ([]osv.Advisory, error) {
	var specific DatabaseSpecific
	if err := json.Unmarshal(entry.DatabaseSpecific, &specific); err != nil {
		return nil, xerrors.Errorf("JSON decode error: %w", err)
	}

	severity := convertSeverity(specific.Severity)
	for i, adv := range advisories {
		advisories[i].Severity = severity

		// Replace a git URL with a CocoaPods package name in a Swift vulnerability
		// and store it as a CocoaPods vulnerability.
		if adv.Ecosystem == vulnerability.Swift {
			adv.Severity = severity
			adv.Ecosystem = vulnerability.Cocoapods
			for _, pkgName := range t.cocoaPodsSpecs[adv.PkgName] {
				adv.PkgName = pkgName
				advisories = append(advisories, adv)
			}
		}
	}

	return advisories, nil
}

func convertSeverity(severity string) types.Severity {
	switch severity {
	case "LOW":
		return types.SeverityLow
	case "MODERATE":
		return types.SeverityMedium
	case "HIGH":
		return types.SeverityHigh
	case "CRITICAL":
		return types.SeverityCritical
	default:
		return types.SeverityUnknown
	}
}
