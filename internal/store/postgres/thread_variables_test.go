package postgres

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

func TestPrepareThreadVariablesSearchQuery_WithCallerID(t *testing.T) {
	t.Parallel()

	caller := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.GetThreadVariablesQuery{
		ThreadIDs: uuid.UUIDs{thread},
		CallerID:  caller,
	}

	sql, _, err := prepareThreadVariablesSearhcQuery(query)
	require.NoError(t, err)

	// Check that the SQL contains both thread_dialog and thread_preview
	assert.True(t, strings.Contains(sql, "im_thread.thread_dialog"), "SQL should contain thread_dialog")
	assert.True(t, strings.Contains(sql, "im_thread.thread_preview"), "SQL should contain thread_preview")
}

func TestPrepareThreadVariablesSearchQuery_WithDomainID(t *testing.T) {
	t.Parallel()

	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.GetThreadVariablesQuery{
		ThreadIDs: uuid.UUIDs{thread},
		DomainID:  1,
	}

	sql, _, err := prepareThreadVariablesSearhcQuery(query)
	require.NoError(t, err)

	// Check that the SQL contains the domain filter
	assert.True(t, strings.Contains(sql, "from im_thread.thread where domain_id"), "SQL should contain domain filter")
}

func TestPrepareThreadVariablesSearchQuery_WithoutCallerAndDomain(t *testing.T) {
	t.Parallel()

	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.GetThreadVariablesQuery{
		ThreadIDs: uuid.UUIDs{thread},
		CallerID:  uuid.Nil,
		DomainID:  0,
	}

	sql, _, err := prepareThreadVariablesSearhcQuery(query)
	require.NoError(t, err)

	// Neither thread_dialog nor thread_preview should be in the SQL
	assert.False(t, strings.Contains(sql, "thread_dialog acl"), "SQL should not contain access control")
	assert.False(t, strings.Contains(sql, "thread_preview"), "SQL should not contain thread_preview")
}

func TestPrepareThreadVariablesLocateQuery_WithCallerID(t *testing.T) {
	t.Parallel()

	caller := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.LocateThreadVariablesQuery{
		ThreadID: thread,
		CallerID: caller,
	}

	sql, _ := prepareThreadVariablesLocateQuery(query)

	// CallerID is validated at the service level, not in the query builder
	// Just verify the thread_id filter is present
	assert.True(t, strings.Contains(sql, "thread_id"), "SQL should contain thread_id filter")
}

func TestPrepareThreadVariablesLocateQuery_WithDomainID(t *testing.T) {
	t.Parallel()

	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.LocateThreadVariablesQuery{
		ThreadID: thread,
		DomainID: 1,
	}

	sql, _ := prepareThreadVariablesLocateQuery(query)

	// Check that the SQL contains the domain filter
	assert.True(t, strings.Contains(sql, "from im_thread.thread where domain_id"), "SQL should contain domain filter")
}

func TestPrepareThreadVariablesLocateQuery_WithoutCallerAndDomain(t *testing.T) {
	t.Parallel()

	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	query := model.LocateThreadVariablesQuery{
		ThreadID: thread,
		CallerID: uuid.Nil,
		DomainID: 0,
	}

	sql, _ := prepareThreadVariablesLocateQuery(query)

	// No domain filter should be present
	assert.False(t, strings.Contains(sql, "from im_thread.thread where domain_id"), "SQL should not contain domain filter")
	// But the thread_id filter should always be present
	assert.True(t, strings.Contains(sql, "thread_id"), "SQL should contain thread_id filter")
}
