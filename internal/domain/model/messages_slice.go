package model

import (
	"time"

	"github.com/google/uuid"
)

type (
	MessageSlice []*Message

	MessageCursor struct {
		CreatedAt time.Time
		Id        uuid.UUID
		Direction bool
	}

	Paging struct {
		After  *MessageCursor
		Before *MessageCursor
	}
)

func NewMessageCursor(id uuid.UUID, createdAt time.Time, direction bool) *MessageCursor {
	return &MessageCursor{
		CreatedAt: createdAt,
		Id:        id,
		Direction: direction,
	}
}

func NewPaging(before, after *MessageCursor) *Paging {
	return &Paging{
		After:  after,
		Before: before,
	}
}

// GetPaging returns a Paging object based on the given MessageSlice.
//
// The returned Paging object will contain the oldest and/or newest message
// in the slice, depending on the given hasMore and isBackward flags.
// If hadCursor is false, the returned Paging object will only contain the oldest
// message if hasMore is true, and the newest message if hasMore is false.
// If hadCursor is true, the returned Paging object will contain the oldest and
// newest messages if hasMore is true, and only the oldest message if hasMore is
// false.
//
// The Direction field of the MessageCursor objects will be set to false if the
// message is the oldest in the slice, and true if the message is the newest in
// the slice.
func (ms *MessageSlice) GetPaging(hasMore, isBackward, hadCursor bool) *Paging {
	msLen := len(*ms)
	if msLen == 0 {
		return nil
	}

	slice := *ms

	var oldest, newest *Message

	if isBackward {
		newest = slice[0]
		oldest = slice[msLen-1]
	} else {
		oldest = slice[0]
		newest = slice[msLen-1]
	}

	paging := new(Paging)

	if !hadCursor {
		if hasMore {
			paging.Before = NewMessageCursor(oldest.ID, oldest.CreatedAt, false)
		}

		return paging
	}

	if isBackward {
		paging.After = NewMessageCursor(newest.ID, newest.CreatedAt, true)

		if hasMore {
			paging.Before = NewMessageCursor(oldest.ID, oldest.CreatedAt, false)
		}
	} else {
		paging.Before = NewMessageCursor(oldest.ID, oldest.CreatedAt, false)

		if hasMore {
			paging.After = NewMessageCursor(newest.ID, newest.CreatedAt, true)
		}
	}

	return paging
}
