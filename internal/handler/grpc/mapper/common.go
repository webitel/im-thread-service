package mapper

import (
	"github.com/google/uuid"
	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

func ParseOptionalUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}

	id, err := uuid.Parse(s)
	if err != nil {
		nilID := uuid.Nil

		return &nilID
	}

	return &id
}

// parseCallerID parses a caller_id from a proto string field.
// Empty string returns uuid.Nil, nil (trusted internal call).
// Parse errors return InvalidArgument.
// Parsed uuid.Nil returns InvalidArgument (nil uuid must not silently mean "trusted internal").
func parseCallerID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.InvalidArgument("invalid caller_id format", errors.WithCause(err))
	}

	if id == uuid.Nil {
		return uuid.Nil, errors.InvalidArgument("caller_id must not be the nil uuid")
	}

	return id, nil
}

func convertToUUIDs(in []string) (uuid.UUIDs, error) {
	out := make(uuid.UUIDs, len(in))
	for i, id := range in {
		converted, err := uuid.Parse(id)
		if err != nil {
			return nil, err
		}

		out[i] = converted
	}

	return out, nil
}

func MapEntitiesFromProto(in []*impb.Entity) []shared.Entity {
	if len(in) == 0 {
		return nil
	}

	out := make([]shared.Entity, 0, len(in))
	for _, e := range in {
		if e == nil {
			continue
		}

		out = append(out, shared.Entity{
			Type:   e.GetType(),
			Offset: int(e.GetOffset()),
			Length: int(e.GetLength()),
			Value:  e.GetValue(),
		})
	}

	return out
}
