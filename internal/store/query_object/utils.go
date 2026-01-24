package queryobject

import (
	"strings"

	"github.com/Masterminds/squirrel"
)

func joinCTE(ctes []squirrel.Sqlizer) string {
	var (
		parts []string
	)

	for _, c := range ctes {
		s, _, _ := c.ToSql()
		parts = append(parts, s)
	}

	return strings.Join(parts, ", ")
}

func formatSort(sort string, allowedFields map[string]bool) (string, bool) {
	if len(sort) < 2 {
		return DefaultSortField, true
	}

	var (
		isDesc = true
		column = sort[1:]
	)

	if sort[0] == '+' {
		isDesc = false
	}

	if !allowedFields[column] {
		return DefaultSortField, true
	}

	return column, isDesc
}
