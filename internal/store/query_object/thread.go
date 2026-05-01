package queryobject

import (
	"fmt"
	"unicode/utf8"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

const (
	threadAlias                   string = "t"
	threadThreadDialogAlias       string = "td"
	threadDirectSettingsAlias     string = "ds"
	threadMembersFullLateralAlias string = "m"
)

const (
	threadLinkThreadDialog = 1 << iota
	threadLinkDirectSettings
	threadLinkMembersLateral
	threadLinkFullMembersLateral
	threadLinkLastMessageLateral
	threadLinkVariables
)

const variablesBuild = `
	case when v.thread_id is null
		then null
	else jsonb_build_object(
		'variables', v.variables
	) end as variables
`

type threadQueryObject struct {
	*baseQueryObject[*threadQueryObject]
	mustIncludeComputedSubject bool
}

func NewThreadQueryObject() *threadQueryObject {
	from := fmt.Sprintf("%s %s", ThreadTable, threadAlias)

	queryObj := new(threadQueryObject)
	queryObj.baseQueryObject = newBaseQueryObject(from, queryObj)

	return queryObj
}

// members means full entity
// member_ids for lazy loading
func (q *threadQueryObject) DefaultFields() []string {
	return []string{
		"id", "domain_id", "created_at", "updated_at",
		"kind", "owner", "subject", "description", "members",
	}
}

func (q *threadQueryObject) FieldsMetadata() map[string]fieldMetadata {
	var (
		resolveSubjectJoin = func() int {
			if q.mustIncludeComputedSubject {
				return threadLinkDirectSettings
			}

			return 0
		}

		resolveSubjectFieldExpression = func() string {
			if !q.mustIncludeComputedSubject {
				return fmt.Sprintf("%s.subject as subject", threadAlias)
			}

			return fmt.Sprintf(`
				coalesce(case
					when %s.kind = %d then %s.title
					else %s.subject
				end, '') as subject
			`, threadAlias, model.ThreadDirect, threadDirectSettingsAlias, threadAlias)
		}
	)
	return map[string]fieldMetadata{
		"id": {
			sqlExpr:      "t.id",
			aliasedExpr:  "t.id as id",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.id",
		},
		"domain_id": {
			sqlExpr:      "t.domain_id",
			aliasedExpr:  "t.domain_id as domain_id",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.domain_id",
		},
		"created_at": {
			sqlExpr:      "t.created_at",
			aliasedExpr:  "t.created_at as created_at",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.created_at",
		},
		"updated_at": {
			sqlExpr:      "t.updated_at",
			aliasedExpr:  "t.updated_at as updated_at",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.updated_at",
		},
		"kind": {
			sqlExpr:      "t.kind",
			aliasedExpr:  "t.kind as kind",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.kind",
		},
		"owner": {
			sqlExpr:      "t.owner",
			aliasedExpr:  "t.owner as owner",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.owner",
		},
		"subject": {
			sqlExpr:      "t.subject",
			aliasedExpr:  resolveSubjectFieldExpression(),
			requiresJoin: resolveSubjectJoin(),
			sortable:     true,
			filterExpr:   "t.subject",
		},
		"description": {
			sqlExpr:      "t.description",
			aliasedExpr:  "t.description as description",
			requiresJoin: 0,
			sortable:     true,
			filterExpr:   "t.description",
		},
		"members": {
			sqlExpr:      "m.members_data",
			aliasedExpr:  "coalesce(m.members_data, '[]'::jsonb) as members",
			requiresJoin: threadLinkFullMembersLateral,
			sortable:     false,
			filterExpr:   "m.members_data",
		},
		"last_msg": {
			sqlExpr:      "msg.last_msg as last_msg",
			aliasedExpr:  "msg.last_msg as last_msg",
			requiresJoin: threadLinkLastMessageLateral,
			sortable:     false,
			filterExpr:   "",
		},
		"last_msg_at": {
			sqlExpr:      "msg.id",
			aliasedExpr:  "msg.id as last_message_id",
			requiresJoin: threadLinkLastMessageLateral,
			sortable:     true,
			filterExpr:   "",
		},
		"variables": {
			sqlExpr:      "v.variables",
			aliasedExpr:  variablesBuild,
			requiresJoin: threadLinkVariables,
			sortable:     false,
			filterExpr:   "v.variables",
		},
	}
}

func (q *threadQueryObject) EnsureJoins(requiredJoin int) {
	if requiredJoin&threadLinkThreadDialog != 0 {
		q.linkThreadDialog()
	}

	if requiredJoin&threadLinkDirectSettings != 0 {
		q.linkDirectSettings()
	}

	if requiredJoin&threadLinkFullMembersLateral != 0 {
		q.linkFullMembersLateral()
	}

	if requiredJoin&threadLinkLastMessageLateral != 0 {
		q.linkLastMessageLateral()
	}

	if requiredJoin&threadLinkVariables != 0 {
		q.linkVariables()
	}
}

func (q *threadQueryObject) WithIDFilter(ids ...uuid.UUID) *threadQueryObject {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".id": ids})
	}

	return q
}

func (q *threadQueryObject) WithSubject() *threadQueryObject {
	q.mustIncludeComputedSubject = true
	return q
}

func (q *threadQueryObject) WithDomainIDFilter(ids ...int) *threadQueryObject {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".domain_id": ids})
	}

	return q
}

func (q *threadQueryObject) WithKindFilter(kinds ...model.ThreadKind) *threadQueryObject {
	if len(kinds) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".kind": kinds})
	}

	return q
}

func (q *threadQueryObject) WithOwnerFilter(owners ...uuid.UUID) *threadQueryObject {
	if len(owners) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".owner": owners})
	}

	return q
}

func (q *threadQueryObject) WithSubjectFilter(subject string) *threadQueryObject {
	if subject != "" && utf8.ValidString(subject) {
		escapedSubject := "%" + q.escapeLikePattern(subject) + "%"
		q.mustIncludeComputedSubject = true
		q.builder = q.builder.Where(
			fmt.Sprintf("(%s.subject ilike ? or %s.title ilike ?)", threadAlias, threadDirectSettingsAlias), escapedSubject, escapedSubject,
		)
	}

	return q
}

func (q *threadQueryObject) WithDescriptionFilter(description string) *threadQueryObject {
	if description != "" && utf8.ValidString(description) {
		escapedDescription := "%" + q.escapeLikePattern(description) + "%"
		q.builder = q.builder.Where(fmt.Sprintf("%s.description ilike ?", threadAlias), escapedDescription)
	}

	return q
}

func (q *threadQueryObject) WithContactIDFilter(memberIDs ...uuid.UUID) *threadQueryObject {
	if len(memberIDs) != 0 {
		sub, args, _ := squirrel.
			Select("thread_id").
			From(ThreadDialogTable).
			Where(squirrel.Eq{"member_id": memberIDs}).
			Where(squirrel.Eq{"deleted_at": nil}).
			GroupBy("thread_id").
			Having(fmt.Sprintf("COUNT(DISTINCT member_id) = %d", len(memberIDs))).
			PlaceholderFormat(squirrel.Question).
			ToSql()

		q.builder = q.builder.Where(
			fmt.Sprintf("%s.id IN (%s)", threadAlias, sub),
			args...,
		)

		q.mustIncludeComputedSubject = true
	}

	return q
}

func (q *threadQueryObject) WithoutDeletedAtFilter() *threadQueryObject {
	q.EnsureJoins(threadLinkThreadDialog)
	q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".deleted_at": nil})
	return q
}

func (q *threadQueryObject) linkThreadDialog() {
	if q.join&threadLinkThreadDialog != 0 {
		return
	}

	q.join |= threadLinkThreadDialog

	q.builder = q.builder.InnerJoin(fmt.Sprintf(
		"%s %s on %s.thread_id = %s.id",
		ThreadDialogTable,
		threadThreadDialogAlias,
		threadThreadDialogAlias,
		threadAlias,
	))
}

func (q *threadQueryObject) linkDirectSettings() {
	if q.join&threadLinkDirectSettings != 0 {
		return
	}

	q.linkThreadDialog()

	q.join |= threadLinkDirectSettings

	q.builder = q.builder.LeftJoin(fmt.Sprintf(
		"%s %s on %s.thread_dialog_id = %s.id",
		DirectSettingsTable,
		threadDirectSettingsAlias,
		threadDirectSettingsAlias,
		threadThreadDialogAlias,
	))
}

func (q *threadQueryObject) linkFullMembersLateral() {
	if q.join&threadLinkFullMembersLateral != 0 {
		return
	}

	q.join |= threadLinkFullMembersLateral

	q.builder = q.builder.LeftJoin(fmt.Sprintf(`
		lateral (
			select jsonb_agg(
				jsonb_build_object(
					'id', %[1]s.id,
					'member_id', %[1]s.member_id,
					'created_at', %[1]s.created_at,
					'updated_at', %[1]s.updated_at,
					'role', %[1]s.thread_role
				)
			) as members_data
			from %[4]s %[1]s
			where %[1]s.thread_id = %[5]s.id
			AND %[1]s.deleted_at IS NULL
		) %[6]s on true
	`, threadThreadDialogAlias, threadDirectSettingsAlias, DirectSettingsTable, ThreadDialogTable, threadAlias, threadMembersFullLateralAlias))
}

func (q *threadQueryObject) linkLastMessageLateral() {
	if q.join&threadLinkLastMessageLateral != 0 {
		return
	}

	q.join |= threadLinkLastMessageLateral

	q.builder = q.builder.CrossJoin(`
		lateral (
			select
				m.id,
				jsonb_build_object(
					'id', m.id,
					'sender_id', m.sender_id,
					'type', m.type,
					'body', m.body,
					'metadata', m.metadata,
					'created_at', m.created_at,
					'updated_at', m.updated_at,
					'documents', (
						select jsonb_agg(jsonb_build_object(
							'id', md.id, 'file_id', md.file_id, 'name', md.name,
							'mime', md.mime, 'size', md.size, 'created_at', md.created_at
						))
						from im_message.message_documents md
						where md.message_id = md.id and m.type = 2
					),
					'images', (
						select jsonb_agg(jsonb_build_object(
							'id', mi.id, 'file_id', mi.file_id, 'mime', mi.mime,
							'thumbnails', mi.thumbnails, 'width', mi.width,
                        	'height', mi.height, 'created_at', mi.created_at
						))
						from im_message.message_images mi
						where mi.message_id = m.id and m.type = 3
					)
				) as last_msg
			from im_message.messages m
			where m.thread_id = t.id
			order by m.id desc
			limit 1
		) as msg
	`)
}

func (q *threadQueryObject) linkVariables() {
	if q.join&threadLinkVariables != 0 {
		return
	}

	q.join |= threadLinkVariables

	q.builder = q.builder.LeftJoin(`
		im_thread.thread_variables v
		on v.thread_id = t.id
	`)
}
