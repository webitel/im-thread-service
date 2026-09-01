package queryobject

import (
	"strings"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectMessageFields_ReplyAudit(t *testing.T) {
	t.Parallel()

	caller := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	tests := []struct {
		name        string
		fields      []string
		visibleTo   uuid.UUID
		wantAudit   bool
		wantColumns string
		wantArgs    []any
	}{
		{
			name:        "no caller keeps the masked column",
			fields:      []string{"id", "reply_to"},
			visibleTo:   uuid.Nil,
			wantColumns: "id, reply_to",
		},
		{
			name:      "caller gets the audit column behind a role check",
			fields:    []string{"id", "reply_to"},
			visibleTo: caller,
			wantAudit: true,
			wantArgs:  []any{caller, 2},
		},
		{
			name:        "fields without reply_to are untouched",
			fields:      []string{"id", "body"},
			visibleTo:   caller,
			wantColumns: "id, body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select().From(MessageHistoryView)

			sql, args, err := selectMessageFields(base, tt.fields, tt.visibleTo).ToSql()
			require.NoError(t, err)

			assert.Equal(t, tt.wantArgs, args)

			if !tt.wantAudit {
				assert.Equal(t, normalizeSQL("SELECT "+tt.wantColumns+" FROM "+MessageHistoryView), normalizeSQL(sql))

				return
			}

			assert.Contains(t, sql, CompactSQL(
				`case when exists ( select 1 from `+ThreadDialogTable+` priv
					where priv.thread_id = v_messages.thread_id
					and priv.domain_id = v_messages.domain_id
					and priv.member_id = $1::uuid
					and priv.deleted_at is null
					and priv.thread_role >= $2
				) then v_messages.reply_to_audit else v_messages.reply_to end as reply_to`,
			))
		})
	}
}

func TestMessageHistoryQuery_WithFields_RejectsAuditColumn(t *testing.T) {
	t.Parallel()

	caller := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	thread := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	sql, _, err := NewMessageHistoryQuery().
		WithFields([]string{"id", "reply_to_audit"}).
		WithCallerLimitation(caller, uuid.UUIDs{thread}).
		ToSQL()

	require.NoError(t, err)
	assert.False(t, strings.Contains(sql, "reply_to_audit"))
}
