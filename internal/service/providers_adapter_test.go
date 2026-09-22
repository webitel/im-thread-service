package service

import (
	"reflect"
	"testing"

	"github.com/webitel/im-thread-service/gen/go/provider/v1"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestMapEntitiesToProvider(t *testing.T) {
	t.Run("nil input yields nil", func(t *testing.T) {
		if got := mapEntitiesToProvider(nil); got != nil {
			t.Errorf("mapEntitiesToProvider(nil) = %v, want nil", got)
		}
	})

	t.Run("value-less entity leaves Value unset", func(t *testing.T) {
		got := mapEntitiesToProvider([]shared.Entity{{Type: "BOLD", Offset: 0, Length: 9}})

		want := []*provider.Entity{{Type: "BOLD", Offset: 0, Length: 9}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("mapEntitiesToProvider() = %v, want %v", got, want)
		}
	})

	t.Run("entity with value is carried over", func(t *testing.T) {
		got := mapEntitiesToProvider([]shared.Entity{{Type: "LINK", Offset: 4, Length: 22, Value: "https://example.com/x"}})

		if len(got) != 1 {
			t.Fatalf("expected 1 entity, got %v", got)
		}

		if got[0].GetValue() != "https://example.com/x" {
			t.Errorf("expected Value to carry over, got %v", got[0].Value)
		}
	})
}
