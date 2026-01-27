package queryobject

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func TestNewDirectSettingsQuery(t *testing.T) {
	query := NewDirectSettingsQuery()

	require.NotNil(t, query)
	assert.Equal(t, directSettingsDefaultFields, query.fields)
	assert.Equal(t, 0, query.join)
}

func TestDirectSettingsQuery_WithFields_DefaultFields(t *testing.T) {
	query := NewDirectSettingsQuery()

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	expectedSQL := `SELECT ds.id as id, ds.domain_id as domain_id, ds.created_at as created_at, 
		ds.updated_at as updated_at, ds.thread_dialog_id as thread_dialog_id, ds.title as title 
		FROM im_thread.direct_settings ds`

	assert.Equal(t, normalizeSQL(expectedSQL), normalizeSQL(sql))
}

func TestDirectSettingsQuery_WithFields_CustomFields(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithFields([]string{"id", "title", "domain_id"})

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	expectedSQL := `SELECT ds.id as id, ds.title as title, ds.domain_id as domain_id 
		FROM im_thread.direct_settings ds`

	assert.Equal(t, normalizeSQL(expectedSQL), normalizeSQL(sql))
}

func TestDirectSettingsQuery_WithFields_MemberID_AddsJoin(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithFields([]string{"id", "member_id"})

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	expectedSQL := `SELECT ds.id as id, td.member_id as member_id 
		FROM im_thread.direct_settings ds 
		INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id`

	assert.Equal(t, normalizeSQL(expectedSQL), normalizeSQL(sql))
}

func TestDirectSettingsQuery_WithFields_InvalidFields_Ignored(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithFields([]string{"id", "invalid_field", "title", "another_invalid"})

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	expectedSQL := `SELECT ds.id as id, ds.title as title 
		FROM im_thread.direct_settings ds`

	assert.Equal(t, normalizeSQL(expectedSQL), normalizeSQL(sql))
}

func TestDirectSettingsQuery_WithFields_EmptyFields_UsesDefaults(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithFields([]string{})

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	assert.Contains(t, sql, "ds.id as id")
	assert.Contains(t, sql, "ds.title as title")
}

func TestDirectSettingsQuery_WithFields_DuplicateFields_RemovedDuplicates(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithFields([]string{"id", "title", "id", "title"})

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	expectedSQL := `SELECT ds.id as id, ds.title as title FROM im_thread.direct_settings ds`
	assert.Equal(t, normalizeSQL(expectedSQL), normalizeSQL(sql))
}

func TestDirectSettingsQuery_WithSort_Ascending(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("+created_at")

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	assert.Contains(t, sql, "ORDER BY ds.created_at ASC")
}

func TestDirectSettingsQuery_WithSort_Descending(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("-created_at")

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	assert.Contains(t, sql, "ORDER BY ds.created_at DESC")
}

func TestDirectSettingsQuery_WithSort_MultipleFields(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("+created_at", "-title", "+id")

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	assert.Contains(t, sql, "ORDER BY ds.created_at ASC, ds.title DESC, ds.id ASC")
}

func TestDirectSettingsQuery_WithSort_MemberID_AddsJoin(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("+member_id")

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)

	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id")
	assert.Contains(t, sql, "ORDER BY td.member_id ASC")
}

func TestDirectSettingsQuery_WithSort_InvalidField_Ignored(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("+invalid_field", "+created_at")

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	assert.NotContains(t, sql, "invalid_field")
	assert.Contains(t, sql, "ORDER BY ds.created_at ASC")
}

func TestDirectSettingsQuery_WithSort_InvalidDirection_Ignored(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("*created_at", "+title")

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	assert.NotContains(t, sql, "*created_at")
	assert.Contains(t, sql, "ORDER BY ds.title ASC")
}

func TestDirectSettingsQuery_WithSort_TooShort_Ignored(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithSort("+", "+title")

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	assert.Contains(t, sql, "ORDER BY ds.title ASC")
}

func TestDirectSettingsQuery_WithIDFilter(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	query := NewDirectSettingsQuery().
		WithIDFilter(id1, id2)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 2)

	assert.Contains(t, sql, "WHERE ds.id IN")
	assert.Contains(t, args, id1)
	assert.Contains(t, args, id2)
}

func TestDirectSettingsQuery_WithIDFilter_Empty_NoFilter(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithIDFilter()

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)
	assert.NotContains(t, sql, "WHERE")
}

func TestDirectSettingsQuery_WithDomainIDFilter(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithDomainIDFilter(1, 2, 3)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 3)

	assert.Contains(t, sql, "WHERE ds.domain_id IN")
	assert.Contains(t, args, 1)
	assert.Contains(t, args, 2)
	assert.Contains(t, args, 3)
}

func TestDirectSettingsQuery_WithThreadDialogIDFilter(t *testing.T) {
	threadDialogID := uuid.New()

	query := NewDirectSettingsQuery().
		WithThreadDialogIDFilter(threadDialogID)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 1)

	assert.Contains(t, sql, "WHERE ds.thread_dialog_id IN")
	assert.Contains(t, args, threadDialogID)
}

func TestDirectSettingsQuery_WithThreadIDFilter_AddsJoins(t *testing.T) {
	threadID := uuid.New()

	query := NewDirectSettingsQuery().
		WithThreadIDFilter(threadID)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 1)

	// Перевіряємо що додано обидва JOIN'и
	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id")
	assert.Contains(t, sql, "INNER JOIN im_thread.thread t ON t.id = td.thread_id")
	assert.Contains(t, sql, "WHERE t.id IN")
	assert.Contains(t, args, threadID)
}

func TestDirectSettingsQuery_WithMemberIDFilter_AddsJoin(t *testing.T) {
	memberID := uuid.New()

	query := NewDirectSettingsQuery().
		WithMemberIDFilter(memberID)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 1)

	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id")
	assert.Contains(t, sql, "WHERE td.member_id IN")
	assert.Contains(t, args, memberID)
}

func TestDirectSettingsQuery_WithThreadIDMemberIDFilter(t *testing.T) {
	threadID := uuid.New()
	memberID1 := uuid.New()
	memberID2 := uuid.New()

	query := NewDirectSettingsQuery().
		WithThreadIDMemberIDFilter(threadID, memberID1, memberID2)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 3)

	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id")
	assert.Contains(t, sql, "td.thread_id = $")
	assert.Contains(t, sql, "td.member_id IN")
}

func TestDirectSettingsQuery_WithThreadIDMemberIDFilter_EmptyMembers_NoFilter(t *testing.T) {
	threadID := uuid.New()

	query := NewDirectSettingsQuery().
		WithThreadIDMemberIDFilter(threadID)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	assert.Empty(t, args)
	assert.NotContains(t, sql, "WHERE")
}

func TestDirectSettingsQuery_WithLimit_Valid(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithLimit(50)

	sql, _, err := query.ToSQL()

	require.NoError(t, err)
	assert.Contains(t, sql, "LIMIT 50")
}

func TestDirectSettingsQuery_WithLimit_TooSmall_UsesDefault(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithLimit(0)

	sql, _, err := query.ToSQL()

	require.NoError(t, err)
	assert.Contains(t, sql, "LIMIT 20")
}

func TestDirectSettingsQuery_WithLimit_TooLarge_UsesDefault(t *testing.T) {
	query := NewDirectSettingsQuery().
		WithLimit(200)

	sql, _, err := query.ToSQL()

	require.NoError(t, err)
	assert.Contains(t, sql, "LIMIT 20")
}

func TestDirectSettingsQuery_MultipleFilters(t *testing.T) {
	id := uuid.New()
	memberID := uuid.New()

	query := NewDirectSettingsQuery().
		WithIDFilter(id).
		WithDomainIDFilter(1).
		WithMemberIDFilter(memberID)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 3)

	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td")
	assert.Contains(t, sql, "WHERE")
	assert.Contains(t, sql, "ds.id IN")
	assert.Contains(t, sql, "ds.domain_id IN")
	assert.Contains(t, sql, "td.member_id IN")
}

func TestDirectSettingsQuery_ComplexQuery(t *testing.T) {
	id := uuid.New()
	threadID := uuid.New()

	query := NewDirectSettingsQuery().
		WithFields([]string{"id", "title", "member_id"}).
		WithIDFilter(id).
		WithThreadIDFilter(threadID).
		WithSort("+created_at", "-member_id").
		WithLimit(25)

	sql, args, err := query.ToSQL()

	require.NoError(t, err)
	require.Len(t, args, 2)

	assert.Contains(t, sql, "SELECT ds.id as id, ds.title as title, td.member_id as member_id")

	assert.Contains(t, sql, "INNER JOIN im_thread.thread_dialog td ON td.id = ds.thread_dialog_id")
	assert.Contains(t, sql, "INNER JOIN im_thread.thread t ON t.id = td.thread_id")

	assert.Contains(t, sql, "WHERE")
	assert.Contains(t, sql, "ds.id IN")
	assert.Contains(t, sql, "t.id IN")

	assert.Contains(t, sql, "ORDER BY ds.created_at ASC, td.member_id DESC")

	assert.Contains(t, sql, "LIMIT 25")

	assert.Contains(t, args, id)
	assert.Contains(t, args, threadID)
}

func TestDirectSettingsQuery_JoinAddedOnlyOnce(t *testing.T) {
	memberID := uuid.New()

	query := NewDirectSettingsQuery().
		WithFields([]string{"member_id"}).
		WithMemberIDFilter(memberID).
		WithSort("+member_id")

	sql, _, err := query.ToSQL()

	require.NoError(t, err)

	joinCount := strings.Count(sql, "INNER JOIN im_thread.thread_dialog td")
	assert.Equal(t, 1, joinCount, "JOIN must be added only one time")
}

func TestDirectSettingsQuery_ChainableAPI(t *testing.T) {
	query := NewDirectSettingsQuery()

	result := query.
		WithFields([]string{"id"}).
		WithSort("+created_at").
		WithIDFilter(uuid.New()).
		WithDomainIDFilter(1).
		WithThreadDialogIDFilter(uuid.New()).
		WithThreadIDFilter(uuid.New()).
		WithMemberIDFilter(uuid.New()).
		WithThreadIDMemberIDFilter(uuid.New(), uuid.New()).
		WithLimit(10)

	assert.IsType(t, &DirectSettingsQuery{}, result)
}

func BenchmarkDirectSettingsQuery_Simple(b *testing.B) {
	for b.Loop() {
		query := NewDirectSettingsQuery()
		_, _, _ = query.ToSQL()
	}
}

func BenchmarkDirectSettingsQuery_Complex(b *testing.B) {
	id := uuid.New()
	threadID := uuid.New()
	memberID := uuid.New()

	for b.Loop() {
		query := NewDirectSettingsQuery().
			WithFields([]string{"id", "title", "member_id"}).
			WithIDFilter(id).
			WithThreadIDFilter(threadID).
			WithMemberIDFilter(memberID).
			WithSort("+created_at", "-member_id").
			WithLimit(25)
		_, _, _ = query.ToSQL()
	}
}

func TestDirectSettingsQuery_WithSort_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		sortFields  []string
		expectSQL   []string
		expectNoSQL []string
	}{
		{
			name:       "single ascending",
			sortFields: []string{"+created_at"},
			expectSQL:  []string{"ORDER BY ds.created_at ASC"},
		},
		{
			name:       "single descending",
			sortFields: []string{"-created_at"},
			expectSQL:  []string{"ORDER BY ds.created_at DESC"},
		},
		{
			name:       "multiple fields",
			sortFields: []string{"+created_at", "-title"},
			expectSQL:  []string{"ORDER BY ds.created_at ASC, ds.title DESC"},
		},
		{
			name:        "invalid direction",
			sortFields:  []string{"*created_at"},
			expectNoSQL: []string{"ORDER BY"},
		},
		{
			name:        "invalid field",
			sortFields:  []string{"+invalid_field"},
			expectNoSQL: []string{"ORDER BY"},
		},
		{
			name:        "empty",
			sortFields:  []string{},
			expectNoSQL: []string{"ORDER BY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := NewDirectSettingsQuery().
				WithSort(tt.sortFields...)

			sql, _, err := query.ToSQL()
			require.NoError(t, err)

			for _, expected := range tt.expectSQL {
				assert.Contains(t, sql, expected)
			}

			for _, notExpected := range tt.expectNoSQL {
				assert.NotContains(t, sql, notExpected)
			}
		})
	}
}
