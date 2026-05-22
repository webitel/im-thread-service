package mapper

import "github.com/google/uuid"

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
