package queryobject

import (
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

const (
	searchLeftLinkMembersLateral = 1 << iota
	searchLeftLinkLastMessageLateral
)

type searchLeftQueryObject struct {
	*baseQueryObject[*searchLeftQueryObject]
}

func NewSearchLeftQueryObject(memberID uuid.UUID) *searchLeftQueryObject {
	from := fmt.Sprintf("%s %s", ThreadTable, threadAlias)

	obj := new(searchLeftQueryObject)
	obj.baseQueryObject = newBaseQueryObject(from, obj)

	obj.builder = obj.builder.Prefix(fmt.Sprintf(`
		WITH latest_membership AS (
			SELECT
				thread_id,
				member_id,
				created_at AS member_since,
				deleted_at AS left_at
			FROM %s
			WHERE member_id = ?
			  AND deleted_at IS NOT NULL
			ORDER BY deleted_at DESC
		)
	`, ThreadDialogTable), memberID)

	obj.builder = obj.builder.
		InnerJoin("latest_membership lm ON lm.thread_id = t.id")

	return obj
}

func (q *searchLeftQueryObject) DefaultFields() []string {
	return []string{
		"id", "domain_id", "created_at", "updated_at",
		"kind", "owner", "subject", "description", "members", "last_msg",
	}
}

func (q *searchLeftQueryObject) FieldsMetadata() map[string]fieldMetadata {
	return map[string]fieldMetadata{
		"id": {
			sqlExpr:     "t.id",
			aliasedExpr: "t.id as id",
			sortable:    true,
		},
		"domain_id": {
			sqlExpr:     "t.domain_id",
			aliasedExpr: "t.domain_id as domain_id",
			sortable:    true,
		},
		"created_at": {
			sqlExpr:     "t.created_at",
			aliasedExpr: "t.created_at as created_at",
			sortable:    true,
		},
		"updated_at": {
			sqlExpr:     "t.updated_at",
			aliasedExpr: "t.updated_at as updated_at",
			sortable:    true,
		},
		"kind": {
			sqlExpr:     "t.kind",
			aliasedExpr: "t.kind as kind",
			sortable:    true,
		},
		"owner": {
			sqlExpr:     "t.owner",
			aliasedExpr: "t.owner as owner",
			sortable:    true,
		},
		"subject": {
			sqlExpr:     "t.subject",
			aliasedExpr: "t.subject as subject",
			sortable:    true,
		},
		"description": {
			sqlExpr:     "t.description",
			aliasedExpr: "t.description as description",
			sortable:    true,
		},
		"members": {
			sqlExpr:      "m.members_data",
			aliasedExpr:  "coalesce(m.members_data, '[]'::jsonb) as members",
			requiresJoin: searchLeftLinkMembersLateral,
			sortable:     false,
		},
		"last_msg": {
			sqlExpr:      "msg.last_msg",
			aliasedExpr:  "msg.last_msg as last_msg",
			requiresJoin: searchLeftLinkLastMessageLateral,
			sortable:     false,
		},
		"deleted_at": {
			sqlExpr:     "lm.left_at",
			aliasedExpr: "lm.left_at as deleted_at",
			sortable:    true,
		},
	}
}

func (q *searchLeftQueryObject) EnsureJoins(requiredJoin int) {
	if requiredJoin&searchLeftLinkMembersLateral != 0 {
		q.linkMembersLateral()
	}
	if requiredJoin&searchLeftLinkLastMessageLateral != 0 {
		q.linkLastMessageLateral()
	}
}

func (q *searchLeftQueryObject) ToSql() (string, []any, error) {
	if len(q.sortFields) == 0 {
		q.builder = q.builder.OrderBy("lm.left_at DESC")
	}
	return q.baseQueryObject.ToSql()
}

func (q *searchLeftQueryObject) WithDomainIDFilter(id int) *searchLeftQueryObject {
	if id != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"t.domain_id": id})
	}
	return q
}

func (q *searchLeftQueryObject) WithKindFilter(kinds ...model.ThreadKind) *searchLeftQueryObject {
	if len(kinds) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"t.kind": kinds})
	}
	return q
}

func (q *searchLeftQueryObject) linkMembersLateral() {
	if q.join&searchLeftLinkMembersLateral != 0 {
		return
	}
	q.join |= searchLeftLinkMembersLateral
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
			from %[2]s %[1]s
			where %[1]s.thread_id = %[3]s.id
			  and %[1]s.deleted_at is null
		) m on true
	`, threadThreadDialogAlias, ThreadDialogTable, threadAlias))
}

func (q *searchLeftQueryObject) linkLastMessageLateral() {
	if q.join&searchLeftLinkLastMessageLateral != 0 {
		return
	}
	q.join |= searchLeftLinkLastMessageLateral
	q.builder = q.builder.LeftJoin(`
		lateral (
			select jsonb_build_object(
				'id', msg_i.id,
				'sender_id', msg_i.sender_id,
				'type', msg_i.type,
				'body', msg_i.body,
				'metadata', msg_i.metadata,
				'created_at', msg_i.created_at,
				'updated_at', msg_i.updated_at
			) as last_msg
			from im_message.messages msg_i
			where msg_i.thread_id = t.id
			  and msg_i.sender_id = lm.member_id
			  and msg_i.created_at between lm.member_since and lm.left_at
			order by msg_i.created_at desc
			limit 1
		) msg on true
	`)
}
