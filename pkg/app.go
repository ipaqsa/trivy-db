package pkg

import (
	"time"

	"github.com/alt-cloud/trivy-db/pkg/utils"
	"github.com/alt-cloud/trivy-db/pkg/vulnsrc"
	"github.com/urfave/cli"
)

type AppConfig struct{}

func (ac *AppConfig) NewApp(version string) *cli.App {
	app := cli.NewApp()
	app.Name = "trivy-db"
	app.Version = version
	app.Usage = "Trivy DB builder"

	app.Commands = []cli.Command{
		{
			Name:   "build",
			Usage:  "build a database file",
			Action: build,
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "only-update",
					Usage: "update db only specified distribution",
					Value: func() *cli.StringSlice {
						var targets cli.StringSlice
						for _, v := range vulnsrc.All {
							targets = append(targets, string(v.Name()))
						}
						return &targets
					}(),
				},
				cli.StringFlag{
					Name:  "cache-dir",
					Usage: "cache directory path",
					Value: utils.CacheDir(),
				},
				cli.DurationFlag{
					Name:   "update-interval",
					Usage:  "update interval",
					Value:  24 * time.Hour,
					EnvVar: "UPDATE_INTERVAL",
				},
			},
		},
	}

	return app
}
