package queryobject

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
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

func (q *threadQueryObject) DefaultFields() []string {
	return []string{
		"id", "domain_id", "created_at", "updated_at",
		"kind", "owner", "subject", "description", "members", "bot_controller_id", "owner_bot_id",
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
		"bot_controller_id": {
			sqlExpr:      "t.bot_controller_id",
			aliasedExpr:  "t.bot_controller_id as bot_controller_id",
			requiresJoin: 0,
			sortable:     false,
			filterExpr:   "t.bot_controller_id",
		},
		"owner_bot_id": {
			sqlExpr:      "t.owner_bot_id",
			aliasedExpr:  "t.owner_bot_id as owner_bot_id",
			requiresJoin: 0,
			sortable:     false,
			filterExpr:   "t.owner_bot_id",
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
		q.mustIncludeComputedSubject = true
		q.builder = q.builder.Where(
			fmt.Sprintf("(%s.subject ~* ? or %s.title ~* ?)", threadAlias, threadDirectSettingsAlias), subject, subject,
		)
	}

	return q
}

func (q *threadQueryObject) WithDescriptionFilter(description string) *threadQueryObject {
	if description != "" && utf8.ValidString(description) {
		q.builder = q.builder.Where(fmt.Sprintf("%s.description ~* ?", threadAlias), description)
	}

	return q
}

func (q *threadQueryObject) WithContactIDFilter(memberIDs ...uuid.UUID) *threadQueryObject {
	if len(memberIDs) != 0 {
		q.EnsureJoins(threadLinkThreadDialog)
		q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".member_id": memberIDs})

		q.mustIncludeComputedSubject = true
	}

	return q
}

// WithSharedMembersFilter narrows results to threads where selfID is an active member AND every contact in memberIDs is also an active member.
func (q *threadQueryObject) WithSharedMembersFilter(selfID uuid.UUID, memberIDs ...uuid.UUID) *threadQueryObject {
	if selfID == uuid.Nil {
		return q
	}

	q.EnsureJoins(threadLinkThreadDialog)
	q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".member_id": selfID})
	q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".deleted_at": nil})
	q.mustIncludeComputedSubject = true

	if len(memberIDs) == 0 {
		return q
	}

	placeholders := make([]string, len(memberIDs))

	args := make([]any, 0, len(memberIDs)+1)
	for i, id := range memberIDs {
		placeholders[i] = "?"

		args = append(args, id)
	}

	args = append(args, len(memberIDs))

	q.builder = q.builder.Where(
		fmt.Sprintf(`%s.id IN (
			SELECT thread_id FROM %s
			WHERE member_id IN (%s)
			  AND deleted_at IS NULL
			GROUP BY thread_id
			HAVING COUNT(DISTINCT member_id) = ?
		)`, threadAlias, ThreadDialogTable, strings.Join(placeholders, ", ")),
		args...,
	)

	return q
}

func (q *threadQueryObject) WithParticipantsFilter(selfID uuid.UUID, domainIDs []int, participants ...dto.ContactIdentity) *threadQueryObject {
	if selfID == uuid.Nil || len(participants) == 0 {
		return q
	}

	q.EnsureJoins(threadLinkThreadDialog)
	q.mustIncludeComputedSubject = true

	prefixArgs := make([]any, 0, len(domainIDs)+2*len(participants))

	domainPlaceholders := make([]string, len(domainIDs))
	for i, id := range domainIDs {
		domainPlaceholders[i] = "?"

		prefixArgs = append(prefixArgs, id)
	}

	pairPlaceholders := make([]string, len(participants))
	for i, p := range participants {
		pairPlaceholders[i] = "(?, ?)"

		prefixArgs = append(prefixArgs, p.Iss, p.Sub)
	}

	var domainClause string
	if len(domainIDs) != 0 {
		domainClause = fmt.Sprintf("c.domain_id IN (%s) AND ", strings.Join(domainPlaceholders, ", "))
	}

	cte := fmt.Sprintf(`WITH participant_contacts AS (
		SELECT c.id FROM %s c
		WHERE %s(c.issuer_id, c.subject_id) IN (%s)
	)`, ContactTable, domainClause, strings.Join(pairPlaceholders, ", "))

	q.builder = q.builder.Prefix(cte, prefixArgs...)

	q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".member_id": selfID})
	q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".deleted_at": nil})

	q.builder = q.builder.Where(fmt.Sprintf(`%s.id IN (
		SELECT td2.thread_id FROM %s td2
		WHERE td2.member_id IN (SELECT id FROM participant_contacts)
		  AND td2.deleted_at IS NULL
		GROUP BY td2.thread_id
		HAVING COUNT(DISTINCT td2.member_id) = (SELECT COUNT(*) FROM participant_contacts)
	)`, threadAlias, ThreadDialogTable))

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
					'role', %[1]s.thread_role,
					'is_bot', %[1]s.is_bot,
					'auto_leave', %[1]s.auto_leave
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

	q.builder = q.builder.LeftJoin(`
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
						where md.message_id = m.id
						and (m.type = 2 or (m.type=5 and m.interactive->'attachments'->'documents' is not null))
					),
					'images', (
						select jsonb_agg(jsonb_build_object(
							'id', mi.id, 'file_id', mi.file_id, 'mime', mi.mime,
							'thumbnails', mi.thumbnails, 'width', mi.width,
                        	'height', mi.height, 'created_at', mi.created_at
						))
						from im_message.message_images mi
						where mi.message_id = m.id and
						(m.type = 3 or (m.type=5 and m.interactive->'attachments'->'images' is not null))
					),
					'location', (
						select jsonb_build_object(
							'address', ml.address,
							'name', ml.name,
							'latitude', ml.latitude,
							'longitude', ml.longitude
						)
						from im_message.message_locations ml
						where m.type = 6 and m.id=ml.message_id
						limit 1
					),
					'contact', (
						select jsonb_build_object(
							'name', mc.name,
							'phone_number', mc.phone_number,
							'email', mc.email
						)
						from im_message.message_contacts mc
						where m.type = 7 and m.id=mc.message_id
						limit 1
					),
					'system', (
						select jsonb_build_object(
							'type', sm.type,
							'metadata', sm.metadata
							)
						from im_message.system_messages sm
						where m.type = 4 and sm.message_id=m.id
						limit 1
					),
					'interactive', m.interactive
				) as last_msg
			from im_message.messages m
			where m.thread_id = t.id
			  and m.deleted_at is null
			order by m.id desc
			limit 1
		) as msg on true
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
