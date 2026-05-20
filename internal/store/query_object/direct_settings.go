package queryobject

import (
	"fmt"
	"slices"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

const (
	directSettingAlias             string = "ds"
	directSettingThreadAlias       string = "t"
	directSettingThreadDialogAlias string = "td"
)

const (
	directSettingsLinkThreadDialog = 1 << iota
	directSettingsLinkThread
)

const (
	directSettingsThreadDialogFKField string = "thread_dialog_id"
)

var (
	directSettingsFieldsMeta = map[string]fieldMetadata{
		"id": {
			sqlExpr:      "ds.id",
			aliasedExpr:  "ds.id as id",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.id",
		},
		"domain_id": {
			sqlExpr:      "ds.domain_id",
			aliasedExpr:  "ds.domain_id as domain_id",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.domain_id",
		},
		"created_at": {
			sqlExpr:      "ds.created_at",
			aliasedExpr:  "ds.created_at as created_at",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.created_at",
		},
		"updated_at": {
			sqlExpr:      "ds.updated_at",
			aliasedExpr:  "ds.updated_at as updated_at",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.updated_at",
		},
		"thread_dialog_id": {
			sqlExpr:      "ds.thread_dialog_id",
			aliasedExpr:  "ds.thread_dialog_id as thread_dialog_id",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.thread_dialog_id",
		},
		"title": {
			sqlExpr:      "ds.title",
			aliasedExpr:  "ds.title as title",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "ds.title",
		},
		"member_id": {
			sqlExpr:      "td.member_id",
			aliasedExpr:  "td.member_id as member_id",
			requiresJoin: directSettingsLinkThreadDialog,
			sortable:     true,
			filterExpr:   "td.member_id",
		},
	}

	directSettingsDefaultFields = []string{
		"id", "domain_id", "created_at", "updated_at", "thread_dialog_id", "title",
	}
)

type DirectSettingsQuery struct {
	fields     []string
	sortFields []string
	builder    squirrel.SelectBuilder
	join       int
}

func NewDirectSettingsQuery() *DirectSettingsQuery {
	return &DirectSettingsQuery{
		fields:  directSettingsDefaultFields,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select().From(fmt.Sprintf("%s %s", DirectSettingsTable, directSettingAlias)),
	}
}

func (q *DirectSettingsQuery) ensureJoin(requiredJoin int) {
	if requiredJoin&directSettingsLinkThreadDialog != 0 && q.join&directSettingsLinkThreadDialog == 0 {
		q.join |= directSettingsLinkThreadDialog
		q.builder = q.builder.InnerJoin(fmt.Sprintf(
			"%s %s ON %s.id = %s.%s",
			ThreadDialogTable,
			directSettingThreadDialogAlias,
			directSettingThreadDialogAlias,
			directSettingAlias,
			directSettingsThreadDialogFKField,
		))
	}

	if requiredJoin&directSettingsLinkThread == 0 || q.join&directSettingsLinkThread != 0 {
		return
	}

	q.ensureJoin(directSettingsLinkThreadDialog)

	q.join |= directSettingsLinkThread
	q.builder = q.builder.InnerJoin(fmt.Sprintf(
		"%s %s ON %s.id = %s.thread_id",
		ThreadTable,
		directSettingThreadAlias,
		directSettingThreadAlias,
		directSettingThreadDialogAlias,
	))
}

func (q *DirectSettingsQuery) WithFields(fields []string) *DirectSettingsQuery {
	if len(fields) == 0 {
		return q
	}

	validFields := make([]string, 0, len(fields))
	for _, f := range fields {
		if meta, ok := directSettingsFieldsMeta[f]; ok && !slices.Contains(validFields, f) {
			validFields = append(validFields, f)

			q.ensureJoin(meta.requiresJoin)
		}
	}

	q.fields = validFields

	return q
}

func (q *DirectSettingsQuery) WithSort(sortFields ...string) *DirectSettingsQuery {
	if len(sortFields) == 0 {
		return q
	}

	validSortFields := make([]string, 0, len(sortFields))

	for _, sortField := range sortFields {
		if len(sortField) < 2 {
			continue
		}

		direction := sortField[0:1]
		fieldName := sortField[1:]

		meta, ok := directSettingsFieldsMeta[fieldName]
		if !ok || !meta.sortable {
			continue
		}

		if direction != "+" && direction != "-" {
			continue
		}

		q.ensureJoin(meta.requiresJoin)

		validSortFields = append(validSortFields, sortField)
	}

	q.sortFields = validSortFields

	return q
}

func (q *DirectSettingsQuery) WithIDFilter(ids ...uuid.UUID) *DirectSettingsQuery {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{directSettingAlias + ".id": ids})
	}

	return q
}

func (q *DirectSettingsQuery) WithDomainIDFilter(domainIDs ...int) *DirectSettingsQuery {
	if len(domainIDs) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{directSettingAlias + ".domain_id": domainIDs})
	}

	return q
}

func (q *DirectSettingsQuery) WithThreadDialogIDFilter(threadDialogIDs ...uuid.UUID) *DirectSettingsQuery {
	if len(threadDialogIDs) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{directSettingAlias + ".thread_dialog_id": threadDialogIDs})
	}

	return q
}

func (q *DirectSettingsQuery) WithThreadIDFilter(threadIDs ...uuid.UUID) *DirectSettingsQuery {
	if len(threadIDs) != 0 {
		q.ensureJoin(directSettingsLinkThreadDialog)
		q.ensureJoin(directSettingsLinkThread)
		q.builder = q.builder.Where(squirrel.Eq{directSettingThreadAlias + ".id": threadIDs})
	}

	return q
}

func (q *DirectSettingsQuery) WithMemberIDFilter(memberIDs ...uuid.UUID) *DirectSettingsQuery {
	if len(memberIDs) != 0 {
		q.ensureJoin(directSettingsLinkThreadDialog)
		q.builder = q.builder.Where(squirrel.Eq{directSettingThreadDialogAlias + ".member_id": memberIDs})
	}

	return q
}

func (q *DirectSettingsQuery) WithThreadIDMemberIDFilter(threadID uuid.UUID, memberIDs ...uuid.UUID) *DirectSettingsQuery {
	if len(memberIDs) == 0 {
		return q
	}

	q.ensureJoin(directSettingsLinkThreadDialog)

	q.builder = q.builder.Where(squirrel.Eq{
		directSettingThreadDialogAlias + ".thread_id": threadID,
		directSettingThreadDialogAlias + ".member_id": memberIDs,
	})

	return q
}

func (q *DirectSettingsQuery) WithLimit(limit int) *DirectSettingsQuery {
	if limit < 1 || limit > 100 {
		limit = DefaultLimit
	}

	q.builder = q.builder.Limit(uint64(limit))

	return q
}

func (q *DirectSettingsQuery) ToSQL() (string, []any, error) {
	selectExprs := make([]string, 0, len(q.fields))
	for _, field := range q.fields {
		if meta, ok := directSettingsFieldsMeta[field]; ok {
			selectExprs = append(selectExprs, meta.aliasedExpr)
		}
	}

	q.builder = q.builder.Columns(selectExprs...)

	for _, sortField := range q.sortFields {
		direction := sortField[0:1]
		fieldName := sortField[1:]

		meta, ok := directSettingsFieldsMeta[fieldName]
		if !ok {
			continue
		}

		orderDir := "ASC"
		if direction == "-" {
			orderDir = "DESC"
		}

		q.builder = q.builder.OrderBy(fmt.Sprintf("%s %s", meta.sqlExpr, orderDir))
	}

	return q.builder.ToSql()
}
