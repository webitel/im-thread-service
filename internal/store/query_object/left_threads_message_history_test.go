package queryobject

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wantLeftThreadsSystemAllowListPredicate = "(m.type <> $1 OR EXISTS (select 1 from im_message.system_messages sm where sm.message_id = m.id and sm.type = any($2)))"

func TestLeftThreadsMessageHistoryQueryObject_WithSystemMessageAllowList(t *testing.T) {
	t.Parallel()

	noOpSQL, noOpArgs, err := NewLeftThreadsMessageHistoryQueryObject().ToSQL()
	require.NoError(t, err)

	tests := []struct {
		name         string
		allowedTypes []string
		wantNoOp     bool
		wantArgs     []any
	}{
		{
			name:         "nil is not restricted: byte-for-byte identical to not calling the method",
			allowedTypes: nil,
			wantNoOp:     true,
		},
		{
			name:         "non-nil empty blocks all system messages, using the same predicate as the non-empty case",
			allowedTypes: []string{},
			wantArgs:     []any{int(4), []string{}}, // model.MessageTypeSystem; empty (non-nil) allow-list
		},
		{
			name:         "non-empty allow-list restricts to the given subtypes",
			allowedTypes: []string{"user_joined", "user_left"},
			wantArgs:     []any{int(4), []string{"user_joined", "user_left"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sql, args, err := NewLeftThreadsMessageHistoryQueryObject().
				WithSystemMessageAllowList(tt.allowedTypes).
				ToSQL()
			require.NoError(t, err)

			if tt.wantNoOp {
				assert.Equal(t, noOpSQL, sql)
				assert.Equal(t, noOpArgs, args)

				return
			}

			assertSQLContains(t, sql, wantLeftThreadsSystemAllowListPredicate)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

func TestLeftThreadsMessageHistoryQueryObject_WithSystemMessageAllowList_ComposesWithTypesFilter(t *testing.T) {
	t.Parallel()

	thread := uuid.New()

	sql, args, err := NewLeftThreadsMessageHistoryQueryObject().
		WithTypesFilter(1, 2).
		WithThreadIDFilter(thread).
		WithSystemMessageAllowList([]string{"user_joined"}).
		ToSQL()

	require.NoError(t, err)
	// Both predicates should be present, AND-combined.
	assertSQLContains(t, sql, "m.type IN")
	assertSQLContains(t, sql, "(m.type <> $4 OR EXISTS (select 1 from im_message.system_messages sm where sm.message_id = m.id and sm.type = any($5)))")

	// TypesFilter args, ThreadID filter arg, then SystemMessageAllowList args
	require.GreaterOrEqual(t, len(args), 5)
	assert.Equal(t, int(4), args[3]) // model.MessageTypeSystem, from WithSystemMessageAllowList
	assert.Equal(t, []string{"user_joined"}, args[4])
}
