package postgres

import (
	"fmt"
	"slices"

	"github.com/huandu/go-sqlbuilder"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

const (
	maxLimit     = 100
	limitDefault = 20
)

const (
	defaultColTag = "default"
)

func parseSortField(sort string) (model.SortField, error) {
	if len(sort) < 2 {
		return model.SortField{}, errors.InvalidArgument("invaliadate sort string format")
	}

	switch sort[0] {
	case '+':
		return model.SortField{Field: sort[1:], Order: model.SortAsc}, nil
	case '-':
		return model.SortField{Field: sort[1:], Order: model.SortDesc}, nil
	default:
		return model.SortField{}, errors.InvalidArgument(fmt.Sprintf("sort must start with + or -: %q", sort))
	}
}

func applyLimit(sb *sqlbuilder.SelectBuilder, limitVal int) {
	if limitVal > maxLimit {
		limitVal = maxLimit
	}
	if limitVal <= 0 {
		limitVal = limitDefault
	}
	sb.Limit(limitVal + 1)
}

func applySort(sb *sqlbuilder.SelectBuilder, sort string, entity *sqlbuilder.Struct) error {
	if sort == "" {
		return nil
	}

	sf, err := parseSortField(sort)
	if err != nil {
		return err
	}

	if err := validateSortField(sf.Field, entity); err != nil {
		return err
	}

	sb.OrderBy(fmt.Sprintf("%s %s", sf.Field, sf.Order))
	return nil
}

func applyPagination(sb *sqlbuilder.SelectBuilder, p model.Pagination, entity *sqlbuilder.Struct) error {
	applyLimit(sb, p.Limit)
	applyOffset(sb, p.Page, p.Limit)
	return applySort(sb, p.Sort, entity)
}

func applyOffset(sb *sqlbuilder.SelectBuilder, page int, limit int) {
	if page > 1 {
		offset := (page - 1) * limit
		sb.Offset(offset)
	}
}

func validateSortField(field string, entity *sqlbuilder.Struct) error {
	if !slices.Contains(entity.Columns(), field) {
		return errors.InvalidArgument(fmt.Sprintf("invalid sort field: %q", field))
	}
	return nil
}

func buildPage[T any](items []T, limit int) model.Page[T] {
	if limit <= 0 {
		limit = limitDefault
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}

	return model.Page[T]{
		Items:   items,
		HasNext: hasNext,
	}
}

func performColumnsValidation(cols []string, entity *sqlbuilder.Struct) []string {
	entityCols := entity.Columns()

	if len(cols) == 0 {
		return entity.WithTag(defaultColTag).Columns()
	}

	validMap := make(map[string]struct{}, len(entityCols))
	for _, c := range entityCols {
		validMap[c] = struct{}{}
	}

	n := 0
	for _, c := range cols {
		if _, exists := validMap[c]; exists {
			cols[n] = c
			n++
		}
	}
	cols = cols[:n]

	if len(cols) == 0 {
		return entity.WithTag(defaultColTag).Columns()
	}

	return cols
}
