package server

import (
	"testing"

	"github.com/webitel/im-thread-service/config"
	"go.uber.org/fx"
)

func TestValidateApp(t *testing.T) {
	cfg := new(config.Config)
	if err := fx.ValidateApp(MainModule(cfg)); err != nil {
		t.Fatalf("DI container validation failed: %v", err)
	}
}
