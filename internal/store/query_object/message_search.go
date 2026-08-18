package queryobject

import (
	"slices"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type MessageSearchQuery struct {
	base         sq.SelectBuilder
	fields       []string
	paginatorCfg Config[MessageHistoryCursor]

	pag *SquirrelPaginator[MessageHistoryCursor]
}

func NewMessageSearchQuery() *MessageSearchQuery {
	return &MessageSearchQuery{
		base: sq.StatementBuilder.
			PlaceholderFormat(sq.Dollar).
			Select().
			From(MessageHistoryView).
			Where("deleted_at is null"),
		pag: New[MessageHistoryCursor](),
	}
}

func (q *MessageSearchQuery) WithFields(fields []string) *MessageSearchQuery {
	if len(fields) == 0 {
		q.fields = defaultFields

		return q
	}

	valid := make([]string, 0, len(fields))
	for _, f := range fields {
		if availableFields[f] && !slices.Contains(valid, f) {
			valid = append(valid, f)
		}
	}

	q.fields = valid

	return q
}

func (q *MessageSearchQuery) WithTermFilter(term string) *MessageSearchQuery {
	term = strings.TrimSpace(term)
	if term == "" {
		return q
	}

	q.base = q.base.Where("body ilike ?", LikeContains(term))

	return q
}

func (q *MessageSearchQuery) WithDomainIDFilter(domainID int) *MessageSearchQuery {
	if domainID > 0 {
		q.base = q.base.Where(sq.Eq{"domain_id": domainID})
	}

	return q
}

func (q *MessageSearchQuery) WithThreadIDsFilter(threadIDs ...uuid.UUID) *MessageSearchQuery {
	if len(threadIDs) != 0 {
		q.base = q.base.Where(sq.Eq{"thread_id": threadIDs})
	}

	return q
}

func (q *MessageSearchQuery) WithSenderIDsFilter(senderIDs ...uuid.UUID) *MessageSearchQuery {
	if len(senderIDs) != 0 {
		q.base = q.base.Where(sq.Eq{"sender_id": senderIDs})
	}

	return q
}

func (q *MessageSearchQuery) WithTypeFilter(types ...int) *MessageSearchQuery {
	if len(types) != 0 {
		q.base = q.base.Where(sq.Eq{"type": types})
	}

	return q
}

func (q *MessageSearchQuery) WithCallerScope(callerID uuid.UUID) *MessageSearchQuery {
	if callerID == uuid.Nil {
		return q
	}

	q.base = q.base.Where(`
		exists (
			select 1
			from `+ThreadDialogTable+` acl
			where acl.thread_id = v_messages.thread_id
			and acl.member_id = ?::uuid
			and v_messages.created_at >= acl.created_at
			and (acl.deleted_at is null or v_messages.created_at <= acl.deleted_at)
		)
	`, callerID)

	return q
}

func (q *MessageSearchQuery) WithCursor(cursor *dto.HistoryMessageCursor) *MessageSearchQuery {
	if cursor == nil {
		return q
	}

	cfg, err := NewMessageHistoryConfigFromRaw(
		uint64(q.limitOrDefault()),
		MessageHistoryCursor{
			ID: cursor.ID,
		},
		cursor.Direction,
	)
	if err != nil {
		return q
	}

	q.paginatorCfg = cfg

	return q
}

func (q *MessageSearchQuery) WithLimit(limit int) *MessageSearchQuery {
	if limit > 0 && limit <= 100 {
		q.paginatorCfg.Limit = uint64(limit)
	}

	return q
}

func (q *MessageSearchQuery) ToSQL() (string, []any, error) {
	if len(q.fields) == 0 {
		q.fields = defaultFields
	}

	if q.paginatorCfg.Limit == 0 {
		q.paginatorCfg.Limit = uint64(DefaultLimit)
	}

	if q.paginatorCfg.Codec == nil {
		q.paginatorCfg.Codec = NewJSONBase64Codec[MessageHistoryCursor]()
		q.paginatorCfg.Mapper = MessageHistoryCursorMapper{}
		q.paginatorCfg.Columns = MessageHistoryColumns
		q.paginatorCfg.Direction = DirectionAfter
	}

	decorated, err := q.pag.Apply(q.base.Columns(q.fields...), q.paginatorCfg)
	if err != nil {
		return "", nil, err
	}

	return decorated.ToSql()
}

func (q *MessageSearchQuery) BuildPageInfo(
	rows *[]*model.Message,
	extract CursorExtractor[*model.Message, MessageHistoryCursor],
) (PageInfo[MessageHistoryCursor], error) {
	return BuildPageInfo(rows, q.paginatorCfg, extract)
}

func (q *MessageSearchQuery) limitOrDefault() int {
	if q.paginatorCfg.Limit > 0 {
		return int(q.paginatorCfg.Limit)
	}

	return DefaultLimit
}
