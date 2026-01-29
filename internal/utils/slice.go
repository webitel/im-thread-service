package utils

import (
	"github.com/google/uuid"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

func Map[T, U any](s []T, f func(T) U) []U {
	res := make([]U, len(s))
	for i, v := range s {
		res[i] = f(v)
	}
	return res
}

func Where[T any](s []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range s {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

func IdsParser(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

// ProcessPagination takes a limit and a slice of items,
// and returns a boolean indicating whether pagination is needed, and a slice of items
// limited to the given limit. If the length of items is greater than the limit,
// it returns true and the first limit items. Otherwise it returns false and the original items slice.
func ProcessPagination[T any](limit int, items []T) (bool, []T) {
	if limit <= 0 {
		limit = queryobject.DefaultLimit
	}

	if len(items) > limit {
		return true, items[:limit]
	}

	return false, items
}
