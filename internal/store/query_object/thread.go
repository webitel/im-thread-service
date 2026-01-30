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
	threadMembersLateralAlias     string = "ml" //members lateral
	threadDirectSettingsAlias     string = "ds"
	threadMembersFullLateralAlias string = "m"
)

const (
	threadLinkThreadDialog = 1 << iota
	threadLinkDirectSettings
	threadLinkMembersLateral
	threadLinkFullMembersLateral
)

type (
	threadQueryObject struct {
		*baseQueryObject[*threadQueryObject]
		mustIncludeComputedSubject bool
	}
)

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
		"kind", "owner", "subject", "description", "member_ids",
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
				case
					when %s.kind = %d then %s.title
					else %s.subject
				end as subject
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
		"member_ids": {
			sqlExpr:      "ml.member_ids",
			aliasedExpr:  "ml.member_ids as member_ids",
			requiresJoin: threadLinkMembersLateral,
			sortable:     false,
			filterExpr:   "ml.member_ids",
		},
		"members": {
			sqlExpr:      "m.members_data",
			aliasedExpr:  "coalesce(m.members_data, '[]'::jsonb) as members",
			requiresJoin: threadLinkFullMembersLateral,
			sortable:     false,
			filterExpr:   "m.members_data",
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

	if requiredJoin&threadLinkMembersLateral != 0 {
		q.linkMembersLateral()
	}

	if requiredJoin&threadLinkFullMembersLateral != 0 {
		q.linkFullMembersLateral()
	}
}

func (q *threadQueryObject) WithIDFilter(ids ...uuid.UUID) *threadQueryObject {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".id": ids})
	}

	return q
}

func (q *threadQueryObject) WithDomainIDFilter(ids ...int) *threadQueryObject {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{threadAlias + ".domain_id": ids})
	}

	return q
}

func (q *threadQueryObject) WithKindFilter(kinds ...int) *threadQueryObject {
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
		q.escapeLikePattern(subject)
		q.mustIncludeComputedSubject = true
		q.builder = q.builder.Where(fmt.Sprintf("(%s.subject ilike ? or %s.title ilike ?)", threadAlias, threadDirectSettingsAlias), subject, subject)
	}

	return q
}

func (q *threadQueryObject) WithDescriptionFilter(description string) *threadQueryObject {
	if description != "" && utf8.ValidString(description) {
		q.escapeLikePattern(description)
		q.builder = q.builder.Where(fmt.Sprintf("%s.description ilike ?", threadAlias), description)
	}

	return q
}

func (q *threadQueryObject) WithMemberIDFilter(memberIDs ...uuid.UUID) *threadQueryObject {
	if len(memberIDs) != 0 {
		q.EnsureJoins(threadLinkThreadDialog)
		q.builder = q.builder.Where(squirrel.Eq{threadThreadDialogAlias + ".member_id": memberIDs})

		q.mustIncludeComputedSubject = true
	}

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

func (q *threadQueryObject) linkMembersLateral() {
	if q.join&threadLinkMembersLateral != 0 {
		return
	}

	q.join |= threadLinkMembersLateral

	q.builder = q.builder.LeftJoin(fmt.Sprintf(`
		lateral (
			select array_agg(%s.member_id) as member_ids
			from %s %s
			where %s.thread_id = %s.id 
		) %s on true
	`, threadThreadDialogAlias, ThreadDialogTable, threadThreadDialogAlias, threadThreadDialogAlias, threadAlias, threadMembersLateralAlias))
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
					'id', %[1]s.member_id,
					'direct_settings', (
						select jsonb_build_object(
							'id', %[2]s.id,
							'domain_id', %[2]s.domain_id,
							'created_at', %[2]s.created_at,
							'updated_at', %[2]s.updated_at,
							'title', %[2]s.title
						)
						from %[3]s %[2]s
						where %[2]s.thread_dialog_id = %[1]s.id
						limit 1
					)
				)
			) as members_data
			from %[4]s %[1]s
			where %[1]s.thread_id = %[5]s.id
		) %[6]s on true
	`, threadThreadDialogAlias, threadDirectSettingsAlias, DirectSettingsTable, ThreadDialogTable, threadAlias, threadMembersFullLateralAlias))
}
