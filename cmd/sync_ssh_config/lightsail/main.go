package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/xflash-panda/v2board-op/internal/app/sync_ssh_config"
	"github.com/xflash-panda/v2board-op/internal/pkg/service"
	"github.com/xflash-panda/v2board-op/internal/pkg/sshconfig"
	"github.com/xflash-panda/v2board-op/internal/pkg/util"
)

const (
	Name      = "sync-ssh-config(lightsail)"
	Version   = "0.5.0"
	CopyRight = "XFLASH-PANDA@2026"
)

func init() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}
	cli.ErrWriter = io.Discard
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("version=%s ", Version)
	}
}

func main() {
	var lightsailSrvConfig service.LightsailSrvConfig
	var configPath string
	var apply bool
	var concurrency int
	var logLevel string

	app := &cli.App{
		Name:        Name,
		Version:     Version,
		Copyright:   CopyRight,
		Usage:       "Sync ~/.ssh/config HostName entries with Lightsail public IPs",
		Description: "Matches SSH Host aliases to Lightsail instance names across all regions; dry-run by default.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "aws_key",
				Usage:       "AWS Key (omit to use the AWS default credential chain)",
				EnvVars:     []string{"X_AWS_KEY", "AWS_KEY"},
				Destination: &lightsailSrvConfig.Key,
			},
			&cli.StringFlag{
				Name:        "aws_secret",
				Usage:       "AWS Secret",
				EnvVars:     []string{"X_AWS_SECRET", "AWS_SECRET"},
				Destination: &lightsailSrvConfig.Secret,
			},
			&cli.StringFlag{
				Name:        "config",
				Value:       "~/.ssh/config",
				Usage:       "Path to SSH config file (~ is expanded)",
				EnvVars:     []string{"X_SSH_CONFIG", "SSH_CONFIG"},
				Destination: &configPath,
			},
			&cli.BoolFlag{
				Name:        "apply",
				Value:       false,
				Usage:       "Write changes (without this flag, only print the diff)",
				EnvVars:     []string{"X_APPLY", "APPLY"},
				Destination: &apply,
			},
			&cli.IntFlag{
				Name:        "concurrency",
				Value:       8,
				Usage:       "Maximum concurrent region queries",
				EnvVars:     []string{"X_CONCURRENCY", "CONCURRENCY"},
				Destination: &concurrency,
			},
			&cli.StringFlag{
				Name:        "log_mode",
				Value:       "info",
				Usage:       "Log level: debug | info | error",
				EnvVars:     []string{"X_LOG_LEVEL", "LOG_LEVEL"},
				Destination: &logLevel,
			},
		},
		Before: func(c *cli.Context) error {
			return util.SetupLogger(logLevel)
		},
		Action: func(c *cli.Context) error {
			expanded, err := expandHome(configPath)
			if err != nil {
				return err
			}
			log.Infof("reading SSH config: %s", expanded)
			cfg, err := sshconfig.Parse(expanded)
			if err != nil {
				return err
			}
			log.Infof("parsed %d Host blocks", len(cfg.Blocks))

			lsSrv, err := service.NewLightSailService(&lightsailSrvConfig)
			if err != nil {
				return err
			}
			factory := func(ctx context.Context, region string) (sync_ssh_config.LightsailAPI, error) {
				return lsSrv.GetClientForRegion(ctx, region)
			}
			job := sync_ssh_config.NewSyncJob(factory, concurrency)

			ctx := context.Background()
			log.Info("fetching Lightsail instances across all regions...")
			instances, err := job.FetchInstances(ctx)
			if err != nil {
				return err
			}
			log.Infof("fetched %d Lightsail instances", len(instances))

			updates, unchanged, unmatched := cfg.Diff(instances)
			fmt.Print(sshconfig.RenderDiff(updates, unchanged, unmatched))

			if len(updates) == 0 {
				return nil
			}
			if !apply {
				fmt.Println("(dry-run mode; rerun with --apply to write)")
				return nil
			}
			backup, err := cfg.ApplyAndWrite(expanded, updates)
			if err != nil {
				return err
			}
			fmt.Printf("backup written to: %s\n", backup)
			fmt.Printf("updated %s\n", expanded)
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// expandHome replaces a leading ~ with the current user's home directory.
func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("look up current user: %w", err)
	}
	return filepath.Join(u.HomeDir, strings.TrimPrefix(p, "~")), nil
}
