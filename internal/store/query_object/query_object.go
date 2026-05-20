package queryobject

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Masterminds/squirrel"
)

type QueryObject interface {
	ToSQL() (string, []any, error)
}

type Entity interface {
	DefaultFields() []string
	FieldsMetadata() map[string]fieldMetadata
	EnsureJoins(requiredJoin int)
}

type fieldMetadata struct {
	sqlExpr      string
	aliasedExpr  string
	requiresJoin int
	sortable     bool
	filterExpr   string
}

type baseQueryObject[T Entity] struct {
	builder    squirrel.SelectBuilder
	limit      int
	fields     []string
	sortFields []string
	join       int

	entity T
}

func newBaseQueryObject[T Entity](from string, ent T) *baseQueryObject[T] {
	return &baseQueryObject[T]{
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select().From(from),
		limit:   DefaultLimit,
		join:    0,
		entity:  ent,
	}
}

func (q *baseQueryObject[T]) WithFields(fields []string) T {
	if len(fields) == 0 {
		return q.entity
	}

	validFields := make([]string, 0, len(fields))
	for _, f := range fields {
		if meta, ok := q.entity.FieldsMetadata()[f]; ok && !slices.Contains(validFields, f) {
			validFields = append(validFields, f)

			q.entity.EnsureJoins(meta.requiresJoin)
		}
	}

	q.fields = validFields

	return q.entity
}

func (q *baseQueryObject[T]) WithSort(sortFields ...string) T {
	if len(sortFields) == 0 {
		return q.entity
	}

	validSortFields := make([]string, 0, len(sortFields))

	for _, sortField := range sortFields {
		if len(sortField) < 2 {
			continue
		}

		direction := sortField[0:1]
		fieldName := sortField[1:]

		meta, ok := q.entity.FieldsMetadata()[fieldName]
		if !ok || !meta.sortable {
			continue
		}

		if direction != "+" && direction != "-" {
			continue
		}

		q.entity.EnsureJoins(meta.requiresJoin)

		validSortFields = append(validSortFields, sortField)
	}

	q.sortFields = validSortFields

	return q.entity
}

func (q *baseQueryObject[T]) WithLimit(limit int, quota ...int) T {
	if limit <= 0 {
		limit = DefaultLimit
	}

	if len(quota) != 0 && limit > quota[0] {
		limit = quota[0]
	}

	q.builder = q.builder.Limit(uint64(limit) + 1)

	return q.entity
}

func (q *baseQueryObject[T]) WithOffset(page int) T {
	if page <= 1 {
		return q.entity
	}

	offset := (page - 1) * q.limit

	q.builder = q.builder.Offset(uint64(offset))

	return q.entity
}

func (q *baseQueryObject[T]) escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")

	return s
}

func (q *baseQueryObject[T]) ToSQL() (string, []any, error) {
	if len(q.fields) < 1 {
		q.fields = q.entity.DefaultFields()
	}

	selectExprs := make([]string, 0, len(q.fields))
	for _, field := range q.fields {
		if meta, ok := q.entity.FieldsMetadata()[field]; ok {
			q.entity.EnsureJoins(meta.requiresJoin)
			selectExprs = append(selectExprs, meta.aliasedExpr)
		}
	}

	q.builder = q.builder.Columns(selectExprs...)

	for _, sortField := range q.sortFields {
		direction := sortField[0:1]
		fieldName := sortField[1:]

		meta, ok := q.entity.FieldsMetadata()[fieldName]
		if !ok {
			continue
		}

		orderDir := "ASC"
		if direction == "-" {
			orderDir = "DESC"
		}

		q.builder = q.builder.OrderBy(fmt.Sprintf("%s %s", meta.sqlExpr, orderDir))
	}

	sql, args, err := q.builder.ToSql()
	sql = CompactSQL(sql)

	return sql, args, err
}
