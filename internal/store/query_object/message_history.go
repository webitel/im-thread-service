package queryobject

import (
	"slices"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var (
	availableFields = map[string]bool{
		"id":               true,
		"thread_id":        true,
		"sender_id":        true,
		"type":             true,
		"body":             true,
		"metadata":         true,
		"created_at":       true,
		"updated_at":       true,
		"documents":        true,
		"images":           true,
		"member":           true,
		"interactive":      true,
		"location":         true,
		"contact":          true,
		"system":           true,
		"reacted_metadata": true,
		"reply_to":         true,
		"forward_origin":   true,
		"delivery_status":  true,
		"statuses":         true,
		"reactions":        true,
		"edited":           true,
		"deleted_at":       true,
	}
	defaultFields = []string{
		"id", "thread_id", "sender_id",
		"type", "body", "metadata",
		"created_at", "updated_at", "images", "documents",
		"member", "interactive", "location", "contact", "system", "reacted_metadata",
		"reply_to", "forward_origin",
		"delivery_status", "statuses", "reactions",
		"edited", "deleted_at",
	}
)

type MessageHistoryQuery struct {
	base         sq.SelectBuilder
	fields       []string
	paginatorCfg Config[MessageHistoryCursor]

	pag *SquirrelPaginator[MessageHistoryCursor]
}

func NewMessageHistoryQuery() *MessageHistoryQuery {
	return &MessageHistoryQuery{
		base: sq.StatementBuilder.
			PlaceholderFormat(sq.Dollar).
			Select().
			From(MessageHistoryView),
		pag: New[MessageHistoryCursor](),
	}
}

func (q *MessageHistoryQuery) WithFields(fields []string) *MessageHistoryQuery {
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

func (q *MessageHistoryQuery) WithDomainIDsFilter(domainIDs ...int) *MessageHistoryQuery {
	if len(domainIDs) != 0 && domainIDs[0] != 0 {
		q.base = q.base.Where(sq.Eq{"domain_id": domainIDs})
	}

	return q
}

func (q *MessageHistoryQuery) WithIDsFilter(ids ...uuid.UUID) *MessageHistoryQuery {
	if len(ids) != 0 {
		q.base = q.base.Where(sq.Eq{"id": ids})
	}

	return q
}

func (q *MessageHistoryQuery) WithThreadIDsFilter(threadIDs ...uuid.UUID) *MessageHistoryQuery {
	if len(threadIDs) != 0 {
		q.base = q.base.Where(sq.Eq{"thread_id": threadIDs})
	}

	return q
}

func (q *MessageHistoryQuery) WithSenderIDsFilter(senderIDs ...uuid.UUID) *MessageHistoryQuery {
	if len(senderIDs) != 0 {
		q.base = q.base.Where(sq.Eq{"sender_id": senderIDs})
	}

	return q
}

func (q *MessageHistoryQuery) WithTypeFilter(types ...int) *MessageHistoryQuery {
	if len(types) != 0 {
		q.base = q.base.Where(sq.Eq{"type": types})
	}

	return q
}

func (q *MessageHistoryQuery) WithCursor(cursor *dto.HistoryMessageCursor) *MessageHistoryQuery {
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

func (q *MessageHistoryQuery) WithCallerLimitation(callerID uuid.UUID, threadIDs uuid.UUIDs) *MessageHistoryQuery {
	if callerID == uuid.Nil {
		return q
	}

	q.base = q.base.Where(
		`
		exists (
			select 1
			from im_thread.thread_dialog acl
			where acl.thread_id = any(?::uuid[])
			and acl.member_id = ?::uuid
			and acl.deleted_at is null
		)
	`,
		threadIDs,
		callerID,
	)

	return q
}

func (q *MessageHistoryQuery) WithLimit(limit int) *MessageHistoryQuery {
	if limit > 0 && limit <= 100 {
		q.paginatorCfg.Limit = uint64(limit)
	}

	return q
}

func (q *MessageHistoryQuery) ToSQL() (string, []any, error) {
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

	withColumns := q.base.Columns(q.fields...)

	decorated, err := q.pag.Apply(withColumns, q.paginatorCfg)
	if err != nil {
		return "", nil, err
	}

	return decorated.ToSql()
}

func (q *MessageHistoryQuery) BuildPageInfo(
	rows *[]*model.Message,
	extract CursorExtractor[*model.Message, MessageHistoryCursor],
) (PageInfo[MessageHistoryCursor], error) {
	return BuildPageInfo(rows, q.paginatorCfg, extract)
}

func (q *MessageHistoryQuery) limitOrDefault() int {
	if q.paginatorCfg.Limit > 0 {
		return int(q.paginatorCfg.Limit)
	}

	return DefaultLimit
}
