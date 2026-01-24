package queryobject

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMessageHistoryQuery_Default(t *testing.T) {
	q := NewMessageHistoryQuery()

	sql, args, err := q.ToSql()

	assert.NoError(t, err)
	assert.NotEmpty(t, sql)
	assert.Contains(t, sql, "FROM "+MessageHistoryView)
	assert.Contains(t, sql, "SELECT *")
	assert.Contains(t, sql, "LIMIT")
	assert.Len(t, args, 0)
}

func TestMessageHistoryQuery_WithFields(t *testing.T) {
	q := NewMessageHistoryQuery().
		WithFields([]string{"id", "body", "created_at", "invalid_field"})

	sql, _, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "SELECT id, body, created_at")
	assert.NotContains(t, sql, "invalid_field")
}

func TestMessageHistoryQuery_WithThreadIds(t *testing.T) {
	threadID := uuid.New()

	q := NewMessageHistoryQuery().
		WithThreadIdsFilter(threadID)

	sql, args, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "thread_id IN")
	assert.Len(t, args, 1)
	assert.Equal(t, threadID, args[0].(uuid.UUID))
}

func TestMessageHistoryQuery_WithMultipleFilters(t *testing.T) {
	threadID := uuid.New()
	senderID := uuid.New()

	q := NewMessageHistoryQuery().
		WithThreadIdsFilter(threadID).
		WithSenderIdsFilter(senderID)

	sql, args, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "thread_id IN")
	assert.Contains(t, sql, "sender_id IN")
	assert.Len(t, args, 2)
}

func TestMessageHistoryQuery_SortAsc(t *testing.T) {
	q := NewMessageHistoryQuery().
		WithSort("+created_at")

	sql, _, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "ORDER BY created_at ASC")
	assert.Contains(t, sql, "id ASC")
}

func TestMessageHistoryQuery_WithCursorDesc(t *testing.T) {
	cursorTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cursorID := uuid.New()

	q := NewMessageHistoryQuery()
	q.cursor = &MessageHistoryCursor{
		CreatedAt: cursorTime,
		ID:        cursorID,
	}

	sql, args, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "(created_at < $")
	assert.Contains(t, sql, "(created_at = $")
	assert.Contains(t, sql, "id < $")
	assert.Len(t, args, 3)

	assert.Equal(t, cursorTime, args[0])
	assert.Equal(t, cursorTime, args[1])
	assert.Equal(t, cursorID, uuid.MustParse(args[2].(string)))
}

func TestMessageHistoryQuery_WithCursorAsc(t *testing.T) {
	cursorTime := time.Now()
	cursorID := uuid.New()

	q := NewMessageHistoryQuery().
		WithSort("+created_at")

	q.cursor = &MessageHistoryCursor{
		CreatedAt: cursorTime,
		ID:        cursorID,
	}

	sql, args, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "created_at > $")
	assert.Contains(t, sql, "created_at = $")
	assert.Contains(t, sql, "id > $")
	assert.Len(t, args, 3)
}

func TestMessageHistoryQuery_Limit(t *testing.T) {
	q := NewMessageHistoryQuery()

	sql, _, err := q.ToSql()

	assert.NoError(t, err)
	assert.Contains(t, sql, "LIMIT "+fmt.Sprint(DefaultLimit))
}
