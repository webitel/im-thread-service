package dto

import "github.com/google/uuid"

type AddTagRequest struct {
	ContactID uuid.UUID
	ThreadID  uuid.UUID
	Tag       string
}

type RemoveTagRequest struct {
	TagID     uuid.UUID
	ContactID uuid.UUID
}

type SearchTagsRequest struct {
	ContactID uuid.UUID
	Page      int
	Size      int
}
