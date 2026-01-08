package model

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"golang.org/x/text/unicode/norm"
)

var (
	linkRegex    = regexp.MustCompile(`https?://[^\s/$.?#].[^\s]*`)
	mentionRegex = regexp.MustCompile(`@[\w]+`)
)

// [INPUT_WRAPPERS] to prevent circular dependencies with DTO
type ImageInput struct {
	FileID   string
	MimeType string
	Name     string
}

type DocumentInput struct {
	FileID   string
	MimeType string
	Name     string
	Size     int64
}

func NewTextMessage(threadID uuid.UUID, from Peer, recipients []Peer, text string) *Message {
	cleanText := prepareText(text)

	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  threadID,
		From:      from,
		Text:      cleanText,
		Type:      MessageTypeText,
		Metadata:  buildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	for _, to := range recipients {
		msg.AddEvent(events.MessageCreated{
			MessageID:  msg.ID,
			ThreadID:   msg.ThreadID,
			FromID:     msg.From.ID,
			FromType:   int(msg.From.Type),
			ToID:       to.ID,
			ToType:     int(to.Type),
			Body:       msg.Text,
			Type:       int16(msg.Type),
			OccurredAt: msg.CreatedAt,
		})
	}

	return msg
}

func NewImageMessage(threadID uuid.UUID, from Peer, recipients []Peer, text string, images []ImageInput) *Message {
	cleanText := prepareText(text)

	domainImages := make([]*MessageImage, 0, len(images))
	for _, img := range images {
		fID, _ := strconv.ParseInt(img.FileID, 10, 64)
		domainImages = append(domainImages, &MessageImage{
			FileID: fID,
			Mime:   img.MimeType,
			Name:   img.Name,
		})
	}

	msg := &Message{
		ID:        uuid.New(),
		ThreadID:  threadID,
		From:      from,
		Text:      cleanText,
		Type:      MessageTypeImage,
		Images:    domainImages,
		Metadata:  buildMetadata(cleanText),
		CreatedAt: time.Now().UTC(),
	}

	for _, to := range recipients {
		msg.AddEvent(events.MessageCreated{
			MessageID:  msg.ID,
			ThreadID:   msg.ThreadID,
			FromID:     msg.From.ID,
			FromType:   int(msg.From.Type),
			ToID:       to.ID,
			ToType:     int(to.Type),
			Body:       msg.Text,
			Type:       int16(msg.Type),
			Images:     mapImagesToPayload(msg.Images),
			OccurredAt: msg.CreatedAt,
		})
	}

	return msg
}

// [PRIVATE_MAPPERS] Logic to convert domain models to event payloads
func mapImagesToPayload(imgs []*MessageImage) []events.ImagePayload {
	res := make([]events.ImagePayload, 0, len(imgs))
	for _, i := range imgs {
		res = append(res, events.ImagePayload{
			FileID: i.FileID,
			Mime:   i.Mime,
			Name:   i.Name,
		})
	}
	return res
}

// [CORE_HELPERS]
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
