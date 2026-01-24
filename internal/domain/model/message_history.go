package model

type (
	MessageHistoryType int
)

const (
	UNSPECIFIED_MESSAGE_TYPE MessageHistoryType = iota
	TEXT
	DOCUMENT
	IMAGE
	SYSTEM
)

type (
// HistoryDocument struct {
// 	ID        uuid.UUID `json:"id"`
// 	URL       *url.URL  `json:"url"`
// 	MessageID uuid.UUID `json:"message_id"`
// 	FileID    int       `json:"file_id"`
// 	MIME      string    `json:"mime"`
// 	FileName  string    `json:"file_name"`
// 	Size      int       `json:"size"`
// 	CreatedAt time.Time `json:"created_at"`
// }

// HistoryImage struct {
// 	ID         uuid.UUID         `json:"id"`
// 	URL        *url.URL          `json:"url"`
// 	MIME       string            `json:"mime"`
// 	CreatedAt  time.Time         `json:"created_at"`
// 	Width      int               `json:"width"`
// 	Height     int               `json:"height"`
// 	Thumbnails map[string]string `json:"thumbnails"`
// }

// HistoryMessage struct {
// 	ID         uuid.UUID `json:"id"`
// 	ThreadID   uuid.UUID `json:"thread_id"`
// 	SenderID   uuid.UUID `json:"sender_id"`
// 	ReceiverID uuid.UUID `json:"receiver_id"`

// 	Body      string             `json:"body"`
// 	Images    []*HistoryImage    `json:"images"`
// 	Documents []*HistoryDocument `json:"documents"`
// 	Metadata  map[string]any     `json:"metadata"`
// 	CreatedAt time.Time          `json:"created_at"`
// 	UpdatedAt time.Time          `json:"updated_at"`
// }
)
