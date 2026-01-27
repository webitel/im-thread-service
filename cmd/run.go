package cmd

import (
	"os"

	"github.com/urfave/cli/v2"

	"github.com/webitel/im-thread-service/cmd/migrate"
	"github.com/webitel/im-thread-service/cmd/server"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

func Run() error {
	app := &cli.App{
		Name:  model.ServiceName,
		Usage: "Microservice for Webitel [I]nstant [M]essaging contacts managing.",
		Commands: []*cli.Command{
			server.CMD(),
			migrate.CMD(),
		},
	}

	return app.Run(os.Args)
}
