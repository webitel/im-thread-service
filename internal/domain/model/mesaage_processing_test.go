package model

import (
	"reflect"
	"testing"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestValidateEntityBounds(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		entities []shared.Entity
		want     []shared.Entity
	}{
		{
			name:     "empty input",
			text:     "hello",
			entities: nil,
			want:     nil,
		},
		{
			name:     "negative offset dropped",
			text:     "hello",
			entities: []shared.Entity{{Type: "BOLD", Offset: -1, Length: 2}},
			want:     nil,
		},
		{
			name:     "zero length dropped",
			text:     "hello",
			entities: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 0}},
			want:     nil,
		},
		{
			name:     "offset+length beyond text dropped",
			text:     "hello",
			entities: []shared.Entity{{Type: "BOLD", Offset: 3, Length: 10}},
			want:     nil,
		},
		{
			name:     "exact fit at end is kept",
			text:     "hello",
			entities: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 5}},
			want:     []shared.Entity{{Type: "BOLD", Offset: 0, Length: 5}},
		},
		{
			name:     "valid and invalid mixed keeps only valid",
			text:     "hello world",
			entities: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 5}, {Type: "ITALIC", Offset: 6, Length: 100}},
			want:     []shared.Entity{{Type: "BOLD", Offset: 0, Length: 5}},
		},
		{
			name: "span starting mid multi-byte rune is dropped",
			// "Привіт" - Cyrillic, each letter is 2 bytes. Offset 1 lands inside the first rune.
			text:     "Привіт",
			entities: []shared.Entity{{Type: "BOLD", Offset: 1, Length: 1}},
			want:     nil,
		},
		{
			name:     "span ending mid multi-byte rune is dropped",
			text:     "Привіт",
			entities: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 3}},
			want:     nil,
		},
		{
			name:     "span aligned on rune boundaries is kept",
			text:     "Привіт",
			entities: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 4}},
			want:     []shared.Entity{{Type: "BOLD", Offset: 0, Length: 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateEntityBounds(tt.text, tt.entities)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateEntityBounds(%q, %v) = %v, want %v", tt.text, tt.entities, got, tt.want)
			}
		})
	}
}

func TestBuildMetadata_CallerEntitiesAugmentedByFallback(t *testing.T) {
	text := "Heads up see https://example.com/x @john"
	callerEntities := []shared.Entity{{Type: "BOLD", Offset: 0, Length: 8}}

	got := BuildMetadata(text, callerEntities)
	entities, _ := got["entities"].([]shared.Entity)

	hasType := func(typ string) bool {
		for _, e := range entities {
			if e.Type == typ {
				return true
			}
		}
		return false
	}

	if !hasType("BOLD") {
		t.Errorf("expected caller-supplied BOLD entity to survive, got %v", entities)
	}
	if !hasType("link") {
		t.Errorf("expected regex fallback to still detect the bare link, got %v", entities)
	}
	if !hasType("mention") {
		t.Errorf("expected regex fallback to still detect the mention, got %v", entities)
	}
}

func TestBuildMetadata_FallbackEntityOverlappingCallerSpanIsDropped(t *testing.T) {
	prefix, url := "see ", "https://example.com/x"
	text := prefix + url
	// Caller already reports this exact URL span as a LINK entity (e.g. gateway-detected).
	callerEntities := []shared.Entity{{Type: "LINK", Offset: len(prefix), Length: len(url)}}

	got := BuildMetadata(text, callerEntities)
	entities, _ := got["entities"].([]shared.Entity)

	if len(entities) != 1 {
		t.Fatalf("expected the overlapping regex-detected link to be deduplicated, got %v", entities)
	}
	if entities[0].Type != "LINK" {
		t.Errorf("expected the surviving entity to be the caller-supplied LINK, got %v", entities[0])
	}
}

func TestBuildMetadata_NoEntitiesFallsBackToRegex(t *testing.T) {
	text := "hello @john"

	got := BuildMetadata(text, nil)
	entities, _ := got["entities"].([]shared.Entity)

	if len(entities) != 1 || entities[0].Type != "mention" {
		t.Errorf("expected regex fallback to detect the mention, got %v", entities)
	}
}

func TestDecodeEntitiesFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     []shared.Entity
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			want:     nil,
		},
		{
			name:     "no entities key",
			metadata: map[string]any{"graphemes": 5},
			want:     nil,
		},
		{
			name:     "jsonb round-trip shape ([]interface{} of map[string]interface{} with float64 numbers)",
			metadata: map[string]any{
				"entities": []interface{}{
					map[string]interface{}{"type": "BOLD", "offset": float64(0), "length": float64(4), "value": ""},
				},
			},
			want: []shared.Entity{{Type: "BOLD", Offset: 0, Length: 4}},
		},
		{
			name: "already-typed []shared.Entity (in-process, not DB round-tripped)",
			metadata: map[string]any{
				"entities": []shared.Entity{{Type: "ITALIC", Offset: 2, Length: 3}},
			},
			want: []shared.Entity{{Type: "ITALIC", Offset: 2, Length: 3}},
		},
		{
			name:     "malformed shape decodes to nil rather than erroring",
			metadata: map[string]any{"entities": "not a list"},
			want:     nil,
		},
		{
			name:     "empty entities list",
			metadata: map[string]any{"entities": []interface{}{}},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeEntitiesFromMetadata(tt.metadata)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodeEntitiesFromMetadata(%v) = %v, want %v", tt.metadata, got, tt.want)
			}
		})
	}
}
