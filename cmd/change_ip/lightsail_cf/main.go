package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/xflash-panda/v2board-op/internal/app/change_ip"
	"github.com/xflash-panda/v2board-op/internal/pkg/api"
	"github.com/xflash-panda/v2board-op/internal/pkg/service"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	Name                       = "change-ip(lightsail-cf)"
	Version                    = "0.1.1"
	CopyRight                  = "XFLASH-PANDA@2023"
	LogLevelDebug              = "debug"
	LogLevelError              = "error"
	LogLevelInfo               = "info"
	DefaultJobMaxTryNum    int = 5
	DefaultLightsailRegion     = "ap-northeast-1"
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
	var apiConfig api.Config
	var lightsailCFJobConfig change_ip.LightsailCFJobConfig
	var lightsailSrvConfig service.LightsailSrvConfig
	var cloudflreSrvConfig service.CloudflareSrvConfig

	var logLevel string
	var jobMaxTryNum int
	defaultServerQueryTags := cli.NewStringSlice("lightsail_cf_ap-northeast-1")
	serverQueryTags := cli.NewStringSlice()

	app := &cli.App{
		Name:        Name,
		Version:     Version,
		Copyright:   CopyRight,
		Usage:       "Automatically change IP script for the v2Board",
		Description: "This script is only used for nodes using lightsail and cloudflare dns",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "api",
				Usage:       "Server address",
				EnvVars:     []string{"X_API", "API"},
				Required:    true,
				Destination: &apiConfig.APIHost,
			},
			&cli.StringFlag{
				Name:        "token",
				Usage:       "Token of server API",
				EnvVars:     []string{"X_TOKEN", "TOKEN"},
				Required:    true,
				Destination: &apiConfig.Token,
			},
			&cli.StringSliceFlag{
				Name:        "server_query_tags",
				Value:       defaultServerQueryTags,
				Usage:       "Used for internal query filtering nodes",
				EnvVars:     []string{"X_SERVER_QUERY_TAGS", "QUERY_TAGS"},
				Destination: serverQueryTags,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "cloudflare_domain",
				Usage:       "Cloudflare Domain",
				EnvVars:     []string{"X_CLOUDFLARE_DOMAIN", "cloudflare_domain"},
				Destination: &lightsailCFJobConfig.Domain,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "cloudflare_email",
				Usage:       "Cloudflare Email",
				EnvVars:     []string{"X_CLOUDFLARE_EMAIL", "CLOUDFLARE_EMAIL"},
				Destination: &cloudflreSrvConfig.Email,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "cloudflare_key",
				Usage:       "Cloudflare Key",
				EnvVars:     []string{"X_CLOUDFLARE_KEY", "CLOUDFLARE_KEY"},
				Destination: &cloudflreSrvConfig.Key,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "lightsail_region",
				Value:       DefaultLightsailRegion,
				Usage:       "Configure the region that lightsail queries",
				EnvVars:     []string{"X_LIGHTSAIL_REGION", "LIGHTSAIL_REGION"},
				Destination: &lightsailSrvConfig.Region,
				Required:    false,
			},
			&cli.StringFlag{
				Name:        "aws_key",
				Usage:       "AWS Key",
				EnvVars:     []string{"X_AWS_KEY", "AWS_KEY"},
				Destination: &lightsailSrvConfig.Key,
				Required:    false,
			},
			&cli.StringFlag{
				Name:        "aws_secret",
				Usage:       "AWS Secret Key",
				EnvVars:     []string{"X_AWS_SECRET", "AWS_SECRET"},
				Destination: &lightsailSrvConfig.Secret,
				Required:    false,
			},
			&cli.IntFlag{
				Name:        "max_num",
				Value:       DefaultJobMaxTryNum,
				Usage:       "Maximum number of job attempts",
				EnvVars:     []string{"X_MAX_NUM", "MAX_NUM"},
				Destination: &jobMaxTryNum,
				Required:    false,
			},
			&cli.StringFlag{
				Name:        "log_mode",
				Value:       LogLevelError,
				Usage:       "Log mode",
				EnvVars:     []string{"X_LOG_LEVEL", "LOG_LEVEL"},
				Destination: &logLevel,
				Required:    false,
			},
		},
		Before: func(c *cli.Context) error {
			log.SetFormatter(&log.TextFormatter{})
			if logLevel == LogLevelDebug {
				log.SetFormatter(&log.TextFormatter{
					FullTimestamp: true,
				})
				log.SetLevel(log.DebugLevel)
				log.SetReportCaller(false)
			} else if logLevel == LogLevelInfo {
				log.SetLevel(log.InfoLevel)
			} else if logLevel == LogLevelError {
				log.SetLevel(log.ErrorLevel)
			} else {
				return fmt.Errorf("log mode %s not supported", logLevel)
			}
			return nil
		},
		Action: func(c *cli.Context) error {
			log.Debugln("api host: ", apiConfig.APIHost)
			log.Debugln("api token: ", apiConfig.Token)
			log.Debugln("aws key: ", lightsailSrvConfig.Key)
			log.Debugln("aws secret: ", lightsailSrvConfig.Secret)
			log.Debugln("cloudflare email: ", cloudflreSrvConfig.Email)
			log.Debugln("max num: ", jobMaxTryNum)
			log.Debugln("server query tags: ", serverQueryTags)
			log.Debugln("lightsail region: ", lightsailSrvConfig.Region)

			if logLevel != LogLevelDebug {
				defer func() {
					if e := recover(); e != nil {
						panic(e)
					}
				}()
			}

			s := make(chan os.Signal)
			signal.Notify(s, os.Interrupt, syscall.SIGTERM)
			go func() {
				for {
					<-s
					log.Warningln("The program cannot be interrupted")
				}
			}()

			lightSailSrv, err := service.NewLightSailService(&lightsailSrvConfig)
			if err != nil {
				log.Fatal(err)
			}
			cloudflareSrv, err := service.NewCloudflareService(&cloudflreSrvConfig)
			if err != nil {
				log.Fatal(err)
			}

			lightsailCFJobConfig.QueryTags = serverQueryTags.Value()
			apiClient := api.New(&apiConfig)

			tryNum := 0
			for tryNum < jobMaxTryNum {
				tryNum++
				lightsailCFJob := change_ip.NewLightsailCFJob(&lightsailCFJobConfig, apiClient, lightSailSrv, cloudflareSrv)
				if err := lightsailCFJob.Init(); err != nil {
					log.Fatalf("monitor initialization failure :%s", err)
				}

				log.Infof("job execution times: %d\n", tryNum)
				rerunStatus, runErr := lightsailCFJob.Run()
				if runErr != nil {
					log.Fatal(runErr)
				}
				if rerunStatus == false {
					break
				}
				log.Infoln("job execution completed")
			}

			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
