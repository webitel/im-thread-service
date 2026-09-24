package mapper

import (
	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func ConvertToChatTag(t *model.ThreadTag) *impb.ChatTag {
	if t == nil {
		return nil
	}

	return &impb.ChatTag{
		Id:        t.ID.String(),
		ThreadId:  t.ThreadID.String(),
		ContactId: t.ContactID.String(),
		Tag:       t.Tag,
		CreatedAt: t.CreatedAtUnix(),
	}
}

func ConvertAddThreadTagRequest(req *impb.AddThreadTagRequest) (*dto.AddTagRequest, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("mapper.convert_add_thread_tag_request"))
	}

	contactID, err := uuid.Parse(req.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id", errors.WithCause(err), errors.WithID("mapper.convert_add_thread_tag_request"))
	}

	threadID, err := uuid.Parse(req.GetThreadId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid thread_id", errors.WithCause(err), errors.WithID("mapper.convert_add_thread_tag_request"))
	}

	return &dto.AddTagRequest{
		ContactID: contactID,
		ThreadID:  threadID,
		Tag:       req.GetTag(),
	}, nil
}

func ConvertRemoveThreadTagRequest(req *impb.RemoveThreadTagRequest) (*dto.RemoveTagRequest, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("mapper.convert_remove_thread_tag_request"))
	}

	tagID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid tag id", errors.WithCause(err), errors.WithID("mapper.convert_remove_thread_tag_request"))
	}

	contactID, err := uuid.Parse(req.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id", errors.WithCause(err), errors.WithID("mapper.convert_remove_thread_tag_request"))
	}

	return &dto.RemoveTagRequest{
		TagID:     tagID,
		ContactID: contactID,
	}, nil
}

func ConvertSearchThreadTagsRequest(req *impb.SearchThreadTagsRequest) (*dto.SearchTagsRequest, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil", errors.WithID("mapper.convert_search_thread_tags_request"))
	}

	contactID, err := uuid.Parse(req.GetContactId())
	if err != nil {
		return nil, errors.InvalidArgument("invalid contact_id", errors.WithCause(err), errors.WithID("mapper.convert_search_thread_tags_request"))
	}

	return &dto.SearchTagsRequest{
		ContactID: contactID,
		Page:      int(req.GetPage()),
		Size:      int(req.GetSize()),
	}, nil
}
