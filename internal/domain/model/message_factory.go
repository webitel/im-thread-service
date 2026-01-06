package model

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/norm"
)

var (
	// [PARSERS]
	// Basic regex for identifying links and user mentions
	linkRegex    = regexp.MustCompile(`https?://[^\s/$.?#].[^\s]*`)
	mentionRegex = regexp.MustCompile(`@[\w]+`)
)

// NewTextMessage performs domain-level enrichment and normalization.
// It returns a Message object ready to be persisted.
func NewTextMessage(threadID uuid.UUID, from, to Peer, text string) *Message {
	// [ENRICHMENT]
	// Clean and normalize text before processing metadata
	cleanText := prepareText(text)

	return &Message{
		ThreadId: threadID,
		From:     from,
		To:       to,
		Text:     cleanText,
		Type:     MessageTypeText,
		Metadata: map[string]any{
			// [METADATA]
			// Store pre-calculated entities and real grapheme count for UI/Search
			"entities":  extractEntities(cleanText),
			"graphemes": uniseg.GraphemeClusterCount(cleanText),
		},
	}
}

// prepareText handles NFC normalization and UTF-8 safety.
func prepareText(s string) string {
	s = strings.TrimSpace(s)

	// [UTF8_SAFETY]
	// Strip invalid sequences to prevent downstream processing crashes
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}

	// [NORMALIZATION]
	// Use NFC (Canonical Decomposition, followed by Canonical Composition)
	// This ensures consistent byte representation for emojis and accents
	return norm.NFC.String(s)
}

// extractEntities finds links and mentions within the text.
func extractEntities(text string) []Entity {
	var entities []Entity

	collect := func(re *regexp.Regexp, entityType string) {
		matches := re.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			entities = append(entities, Entity{
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
