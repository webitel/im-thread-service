package mapper

import (
	"github.com/google/uuid"

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
