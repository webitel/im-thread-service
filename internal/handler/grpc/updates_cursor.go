package grpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
)

// UpdatesCursorServer hands out a contact's GetUpdates cursor, e.g. for the socket's connected event.
type UpdatesCursorServer struct {
	impb.UnimplementedUpdatesCursorServer

	cursors UpdatesCursors
}

func NewUpdatesCursorServer(cursors UpdatesCursors) *UpdatesCursorServer {
	return &UpdatesCursorServer{cursors: cursors}
}

func (s *UpdatesCursorServer) Get(ctx context.Context, req *impb.GetUpdatesCursorRequest) (*impb.GetUpdatesCursorResponse, error) {
	if _, err := uuid.Parse(req.GetCallerId()); err != nil {
		return nil, errors.InvalidArgument("invalid caller_id", errors.WithCause(err), errors.WithID("grpc.updates_cursor.caller_id"))
	}

	cursor, err := s.cursors.ContactCursor(ctx, req.GetCallerId())
	if err != nil {
		return nil, err
	}

	return &impb.GetUpdatesCursorResponse{Cursor: cursor}, nil
}
