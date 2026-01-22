package model

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"golang.org/x/text/unicode/norm"
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

func buildMetadata(text string) map[string]any {
	return map[string]any{
		"entities":  extractEntities(text),
		"graphemes": uniseg.GraphemeClusterCount(text),
	}
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
