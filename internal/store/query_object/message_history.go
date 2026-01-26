package queryobject

import (
	"fmt"
	"slices"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

var (
	//change bool type to string, to add jsonb select support?
	//something like document.mime to build jsonb object only with mime field?
	availableFields = map[string]bool{
		"id":          true,
		"thread_id":   true,
		"sender_id":   true,
		"receiver_id": true,
		"type":        true,
		"body":        true,
		"metadata":    true,
		"created_at":  true,
		"updated_at":  true,
		"documents":   true,
		"images":      true,
	}
	defaultFields = []string{
		"id", "thread_id", "sender_id",
		"receiver_id", "type", "body", "metadata", "created_at",
		"updated_at",
	}
)

type (
	MessageHistoryQuery struct {
		builder squirrel.SelectBuilder
		cursor  *MessageHistoryCursor
		desc    bool
		limit   int
		fields  []string
	}

	MessageHistoryCursor struct {
		CreatedAt time.Time `json:"created_at"`
		ID        uuid.UUID `json:"id"`
		Direction bool      `json:"direction"`
	}
)

func NewMessageHistoryCursorFromDTOCursor(cursor *dto.HistoryMessageCursor) *MessageHistoryCursor {
	return &MessageHistoryCursor{
		CreatedAt: cursor.CreatedAt,
		ID:        cursor.Id,
		Direction: cursor.Direction,
	}
}

func NewMessageHistoryQuery() *MessageHistoryQuery {
	return &MessageHistoryQuery{
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select().From(MessageHistoryView),
		desc:    true,
		limit:   DefaultLimit + 1,
	}
}

func (q *MessageHistoryQuery) WithFields(fields []string) *MessageHistoryQuery {
	if len(fields) == 0 {
		q.fields = defaultFields
		return q
	}

	validFields := make([]string, 0, len(fields))
	for _, f := range fields {
		if availableFields[f] && !slices.Contains(validFields, f) {
			validFields = append(validFields, f)
		}
	}

	q.fields = validFields

	return q
}

func (q *MessageHistoryQuery) WithIdsFilter(ids ...uuid.UUID) *MessageHistoryQuery {
	if len(ids) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"id": ids})
	}

	return q
}

func (q *MessageHistoryQuery) WithThreadIdsFilter(threadIds ...uuid.UUID) *MessageHistoryQuery {
	if len(threadIds) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"thread_id": threadIds})
	}

	return q
}

func (q *MessageHistoryQuery) WithSenderIdsFilter(senderIds ...uuid.UUID) *MessageHistoryQuery {
	if len(senderIds) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"sender_id": senderIds})
	}

	return q
}

func (q *MessageHistoryQuery) WithReceiverIdsFilter(receiverIds ...uuid.UUID) *MessageHistoryQuery {
	if len(receiverIds) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"receiver_id": receiverIds})
	}

	return q
}

func (q *MessageHistoryQuery) WithTypeFilter(types ...int) *MessageHistoryQuery {
	if len(types) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{"type": types})
	}

	return q
}

func (q *MessageHistoryQuery) WithCursor(cursor *MessageHistoryCursor) *MessageHistoryQuery {
	orderDir := "DESC"

	if cursor != nil {
		q.cursor = cursor

		if q.cursor.Direction {
			orderDir = "ASC"
		}
	}

	q.builder = q.builder.
		OrderBy(fmt.Sprintf("created_at %s", orderDir)).
		OrderBy(fmt.Sprintf("id %s", orderDir))

	return q
}

func (q *MessageHistoryQuery) WithLimit(limit int) *MessageHistoryQuery {
	if limit > 0 && limit <= 100 {
		q.limit = limit + 1
	}

	return q
}

func (q *MessageHistoryQuery) ToSql() (string, []any, error) {
	if len(q.fields) == 0 {
		q.fields = defaultFields
	}

	q.builder = q.builder.Columns(q.fields...)
	q.builder = q.builder.Limit(uint64(q.limit))

	if q.cursor != nil {
		var op string
		if q.cursor.Direction {
			// For DESC: (created_at, id) < (cursor.created_at, cursor.id)
			op = ">"
		} else {
			// For ASC: (created_at, id) > (cursor.created_at, cursor.id)
			op = "<"
		}

		q.builder = q.builder.Where(
			fmt.Sprintf("(created_at, id) %s (?, ?::uuid)", op),
			q.cursor.CreatedAt,
			q.cursor.ID,
		)
	}

	return q.builder.ToSql()
}
