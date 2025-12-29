package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/webitel/im-thread-service/config"
)

const (
	ServiceName      = "your-service-name"
	ServiceNamespace = "webitel"
)

var (
	version        = "0.0.0"
	commit         = "hash"
	commitDate     = time.Now().String()
	branch         = "branch"
	buildTimestamp = ""
)

func Run() error {
	app := &cli.App{
		Name:  ServiceName,
		Usage: "Microservice for Webitel platform",
		Commands: []*cli.Command{
			serverCmd(),
		},
	}

	return app.Run(os.Args)
}

func serverCmd() *cli.Command {
	return &cli.Command{
		Name:    "server",
		Aliases: []string{"s"},
		Usage:   "Run the gRPC server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config_file",
				Usage: "Path to the configuration file",
			},
		},
		Action: func(c *cli.Context) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return err
			}
			app := NewApp(cfg)

			if err := app.Start(c.Context); err != nil {
				return err
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			<-stop

			slog.Info("Shutting down...")
			return app.Stop(context.Background())
		},
	}
}
