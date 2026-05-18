package model

import (
	"time"

	"github.com/google/uuid"
)

type MessageAttachment interface {
	GetFileID() int64
}

type MessageImage struct {
	ID         uuid.UUID      `json:"id" db:"id"`
	MessageID  uuid.UUID      `json:"message_id" db:"message_id"`
	FileID     int64          `json:"file_id" db:"file_id"`
	Name       string         `json:"name" db:"name"`
	Mime       string         `json:"mime" db:"mime"`
	Width      int32          `json:"width,omitempty" db:"width"`
	Height     int32          `json:"height,omitempty" db:"height"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
	Thumbnails map[string]any `json:"thumbnails,omitempty" db:"thumbnails"`
	URL        string         `json:"url"`
	Size       int64
}

func (mi *MessageImage) GetID() int64        { return mi.FileID }
func (mi *MessageImage) GetURL() string      { return mi.URL }
func (mi *MessageImage) GetName() string     { return mi.Name }
func (mi *MessageImage) GetMimeType() string { return mi.Mime }
func (mi *MessageImage) GetFileID() int64    { return mi.FileID }
func (mi *MessageImage) SetURL(url string)   { mi.URL = url }
func (mi *MessageImage) SetName(name string) { mi.Name = name }
func (mi *MessageImage) SetMime(mime string) { mi.Mime = mime }
func (mi *MessageImage) SetID(id int64)      { mi.FileID = id }
func (mi *MessageImage) SetSize(size int64)  { mi.Size = size }

type MessageDocument struct {
	ID        uuid.UUID `json:"id" db:"id"`
	MessageID uuid.UUID `json:"message_id" db:"message_id"`
	FileID    int64     `json:"file_id" db:"file_id"`
	Name      string    `json:"name" db:"name"`
	Mime      string    `json:"mime" db:"mime"`
	Size      int64     `json:"size" db:"size"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	URL       string    `json:"url,omitempty"`
}

func (md *MessageDocument) GetFileID() int64    { return md.FileID }
func (md *MessageDocument) GetID() int64        { return md.FileID }
func (md *MessageDocument) SetID(id int64)      { md.FileID = id }
func (md *MessageDocument) GetURL() string      { return md.URL }
func (md *MessageDocument) GetName() string     { return md.Name }
func (md *MessageDocument) GetMimeType() string { return md.Mime }
func (md *MessageDocument) SetURL(url string)   { md.URL = url }
func (md *MessageDocument) SetName(name string) { md.Name = name }
func (md *MessageDocument) SetMime(mime string) { md.Mime = mime }
func (md *MessageDocument) SetSize(size int64)  { md.Size = size }
