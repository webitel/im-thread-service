package model

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/norm"

	"github.com/webitel/im-thread-service/internal/domain/shared"
)

var (
	linkRegex    = regexp.MustCompile(`https?://[^\s/$.?#].[^\s]*`)
	mentionRegex = regexp.MustCompile(`@[\w]+`)
)

func prepareText(s string) string {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}

	return norm.NFC.String(s)
}

func BuildMetadata(text string, entities []shared.Entity) map[string]any {
	var entitiesToUse []shared.Entity
	if len(entities) == 0 {
		entitiesToUse = extractEntities(text)
	} else {
		// The regex fallback only detects link/mention spans that the gateway's markdown parser
		// never emits on its own, so it still runs to augment (not replace) caller-supplied
		// entities — otherwise a bare URL or @mention inside an otherwise-formatted message would
		// stop being linkified. Fallback spans overlapping a caller-supplied span are dropped to
		// avoid duplicating an entity the gateway already reported.
		entitiesToUse = make([]shared.Entity, len(entities), len(entities)+2)
		copy(entitiesToUse, entities)
		entitiesToUse = append(entitiesToUse, nonOverlapping(extractEntities(text), entities)...)
	}

	entitiesToUse = validateEntityBounds(text, entitiesToUse)

	return map[string]any{
		"entities":  entitiesToUse,
		"graphemes": uniseg.GraphemeClusterCount(text),
	}
}

// nonOverlapping returns the entities from candidates whose byte span does not overlap any span
// in existing, so a regex-detected fallback entity is only kept where it doesn't duplicate a span
// already reported by the caller.
func nonOverlapping(candidates, existing []shared.Entity) []shared.Entity {
	var result []shared.Entity

	for _, c := range candidates {
		overlaps := false
		for _, e := range existing {
			if c.Offset < e.Offset+e.Length && e.Offset < c.Offset+c.Length {
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, c)
		}
	}

	return result
}

func extractEntities(text string) []shared.Entity {
	var entities []shared.Entity

	collect := func(re *regexp.Regexp, entityType string) {
		matches := re.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			entities = append(entities, shared.Entity{
				Type:   entityType,
				Offset: loc[0],
				Length: loc[1] - loc[0],
				Value:  text[loc[0]:loc[1]],
			})
		}
	}
	collect(linkRegex, "link")
	collect(mentionRegex, "mention")

	return entities
}

func validateEntityBounds(text string, entities []shared.Entity) []shared.Entity {
	if len(entities) == 0 {
		return nil
	}

	textLen := len(text)
	var validEntities []shared.Entity

	for _, e := range entities {
		if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > textLen {
			continue
		}

		// Bounds alone don't guarantee the span lands on rune boundaries — a bounds-valid span
		// can still split a multi-byte UTF-8 character, producing invalid UTF-8 for any renderer
		// slicing text[offset:offset+length].
		if !utf8.RuneStart(text[e.Offset]) {
			continue
		}
		if end := e.Offset + e.Length; end < textLen && !utf8.RuneStart(text[end]) {
			continue
		}

		validEntities = append(validEntities, e)
	}

	return validEntities
}

// DecodeEntitiesFromMetadata recovers []shared.Entity from a Message.Metadata map, whether it
// came back from a Postgres JSONB round-trip (raw entries decode to []interface{} of
// map[string]interface{}, with float64 numbers) or was set in-process as already-typed
// []shared.Entity. Malformed or absent data decodes to nil rather than erroring, since a forward
// with no recoverable entities should fall back to regex-based extraction, not fail.
func DecodeEntitiesFromMetadata(metadata map[string]any) []shared.Entity {
	if metadata == nil {
		return nil
	}

	raw, exists := metadata["entities"]
	if !exists || raw == nil {
		return nil
	}

	if typed, ok := raw.([]shared.Entity); ok {
		return typed
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}

	var entities []shared.Entity
	if err := json.Unmarshal(data, &entities); err != nil {
		return nil
	}

	if len(entities) == 0 {
		return nil
	}

	return entities
}
