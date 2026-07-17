package mapper

import "github.com/google/uuid"

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
