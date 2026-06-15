package main

import (
	"log/slog"

	"github.com/webitel/webitel-go-kit/pkg/semconv"

	"github.com/webitel/im-thread-service/cmd"
)

//go:generate mockery

func main() {
	if err := cmd.Run(); err != nil {
		slog.Error("running cmd", semconv.ErrorKey, err)

		return
	}
}
