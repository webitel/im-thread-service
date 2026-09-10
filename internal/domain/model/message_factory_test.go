package model

import (
	"reflect"
	"testing"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func TestNewDocumentMessage_EntityOffsetsSurviveWhenCallerSuppliesEntities(t *testing.T) {
	// Body has leading/trailing whitespace, which prepareText would normally trim — but the
	// caller-supplied BOLD entity's offsets were measured against this exact raw string by the
	// gateway, so trimming it first would shift "Bold" out from under the span (regression
	// covered here: previously the caption's Body was normalized before/independently of the
	// entities that were computed against the raw text).
	body := " Bold tail"
	entities := []shared.Entity{{Type: "BOLD", Offset: 1, Length: 4}}

	msg := NewDocumentMessage(MessageCreate{Body: body, Entities: entities})

	if msg.Body != body {
		t.Fatalf("expected Body to stay byte-identical to the input when entities are supplied, got %q", msg.Body)
	}

	got, _ := msg.Metadata["entities"].([]shared.Entity)
	if len(got) != 1 || got[0] != entities[0] {
		t.Fatalf("expected the BOLD entity to survive unchanged (still pointing at %q), got %v", body[1:5], got)
	}
	if body[got[0].Offset:got[0].Offset+got[0].Length] != "Bold" {
		t.Fatalf("entity no longer points at \"Bold\" in the stored body: got %q", body[got[0].Offset:got[0].Offset+got[0].Length])
	}
}

func TestNewDocumentMessage_NoEntitiesStillNormalizesBody(t *testing.T) {
	body := "  hello world  "

	msg := NewDocumentMessage(MessageCreate{Body: body})

	if msg.Body != "hello world" {
		t.Fatalf("expected Body to be trimmed when no entities are supplied, got %q", msg.Body)
	}
}

func TestNewDocumentMessage_TrimmedAwayEntityDroppedNotMisaligned(t *testing.T) {
	body := "Bold tail   "
	// Length 7 starting at 5 lands on "tail   " in the raw body, but is out of bounds once the
	// (unused, since entities are supplied) trimmed form would apply — this test only pins that
	// entities are validated against the exact text stored as Body, not silently mismatched.
	entities := []shared.Entity{{Type: "ITALIC", Offset: 5, Length: 7}}

	msg := NewDocumentMessage(MessageCreate{Body: body, Entities: entities})

	got, _ := msg.Metadata["entities"].([]shared.Entity)
	if !reflect.DeepEqual(got, []shared.Entity{{Type: "ITALIC", Offset: 5, Length: 7}}) {
		t.Fatalf("expected the ITALIC entity to remain valid against the untrimmed stored body, got %v", got)
	}
	if msg.Body[5:12] != "tail   " {
		t.Fatalf("entity offset no longer matches stored body: %q", msg.Body[5:12])
	}
}
