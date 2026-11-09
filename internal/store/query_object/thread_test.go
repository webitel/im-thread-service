package queryobject

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/service/dto"
)

func stripSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}

		return r
	}, s)
}

func assertSQLContains(t *testing.T, sql, fragment string) {
	t.Helper()
	assert.Contains(t, stripSpaces(sql), stripSpaces(fragment))
}

func argValues(args []any) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = fmt.Sprintf("%v", a)
	}

	return out
}

func TestThreadQuery_WithAnyDialogScope_OneDialogPerThread(t *testing.T) {
	sql, _, err := NewThreadQueryObject().
		WithSubject().
		WithoutDeletedAtFilter().
		WithAnyDialogScope().
		ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "td.id = ( SELECT d.id FROM im_thread.thread_dialog d WHERE d.thread_id = t.id")
	assertSQLContains(t, sql, "ORDER BY d.is_bot DESC, d.created_at, d.id LIMIT 1")
}

func TestThreadQuery_WithAllMembersFilter(t *testing.T) {
	memberID := uuid.New()

	tests := []struct {
		name      string
		memberIDs []uuid.UUID
		wantSQL   bool
	}{
		{name: "filters by members without the caller", memberIDs: []uuid.UUID{memberID}, wantSQL: true},
		{name: "no members is a no-op", memberIDs: nil, wantSQL: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := NewThreadQueryObject().
				WithAllMembersFilter(tt.memberIDs...).
				ToSQL()
			require.NoError(t, err)

			if !tt.wantSQL {
				assert.NotContains(t, sql, "HAVING")
				assert.Empty(t, args)

				return
			}

			assertSQLContains(t, sql, "HAVING COUNT(DISTINCT member_id) =")
			assert.NotContains(t, stripSpaces(sql), "td.member_id=$")
			assert.Equal(t, []string{memberID.String(), "1"}, argValues(args))
		})
	}
}

func TestThreadQuery_WithAllParticipantsFilter(t *testing.T) {
	tests := []struct {
		name         string
		participants []dto.ContactIdentity
		wantSQL      bool
	}{
		{name: "filters by participants without the caller", participants: []dto.ContactIdentity{{Sub: "sub-1", Iss: "telegram"}}, wantSQL: true},
		{name: "no participants is a no-op", participants: nil, wantSQL: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := NewThreadQueryObject().
				WithAllParticipantsFilter([]int{7}, tt.participants...).
				ToSQL()
			require.NoError(t, err)

			if !tt.wantSQL {
				assert.NotContains(t, sql, "participant_contacts")
				assert.Empty(t, args)

				return
			}

			assertSQLContains(t, sql, "WITH participant_contacts AS (")
			assert.NotContains(t, stripSpaces(sql), "td.member_id=$")
			assert.Equal(t, []string{"7", "telegram", "sub-1"}, argValues(args))
		})
	}
}
