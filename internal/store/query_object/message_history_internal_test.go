package queryobject

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithInternalVisibility_NilCaller_StripsInternalNotes verifies the fail-closed
// behavior: when the caller identity is uuid.Nil we cannot prove the requester is a
// Webitel user, so all internal notes must be hidden (NOT v_messages.internal added).
func TestWithInternalVisibility_NilCaller_StripsInternalNotes(t *testing.T) {
	q := NewMessageHistoryQuery().
		WithInternalVisibility(uuid.Nil)

	sql, args, err := q.ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "NOT v_messages.internal")

	// The nil-caller branch adds no bind parameters — it inlines a literal predicate.
	assert.NotContains(t, argValues(args), uuid.Nil.String(),
		"nil caller branch must not bind uuid.Nil as a parameter")
}

// TestWithInternalVisibility_KnownCaller_UsesACLSubquery verifies that a non-nil
// callerID produces the ACL EXISTS subquery so internal notes are visible to the
// caller when they are a Webitel member of the thread (via IS NULL).
func TestWithInternalVisibility_KnownCaller_UsesACLSubquery(t *testing.T) {
	callerID := uuid.New()

	q := NewMessageHistoryQuery().
		WithInternalVisibility(callerID)

	sql, args, err := q.ToSQL()
	require.NoError(t, err)

	// Must NOT use the blunt strip clause. The nil-caller branch emits the predicate
	// as a standalone term — "(NOT v_messages.internal)" — so stripped it reads
	// "NOTv_messages.internal)". The known-caller branch always continues into an OR,
	// so it reads "NOTv_messages.internalOR...". Assert the blunt, paren-closed form
	// is absent rather than the substring itself, which legitimately appears in the OR.
	assert.NotContains(t, stripSpaces(sql), "NOTv_messages.internal)",
		"known caller must not unconditionally strip internal notes")

	// Must include the conditional ACL subquery.
	assertSQLContains(t, sql, "NOT v_messages.internal OR EXISTS")
	assertSQLContains(t, sql, "acl.via is null")

	// The callerID must be bound as a parameter.
	assert.Contains(t, argValues(args), callerID.String(),
		"callerID must be bound in the ACL subquery")
}

// TestWithInternalVisibility_ComposesWithThreadFilter checks that calling
// WithInternalVisibility after WithThreadIDsFilter does not lose the thread filter.
func TestWithInternalVisibility_ComposesWithThreadFilter(t *testing.T) {
	callerID := uuid.New()
	threadID := uuid.New()

	q := NewMessageHistoryQuery().
		WithThreadIDsFilter(threadID).
		WithInternalVisibility(callerID)

	sql, args, err := q.ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "thread_id")
	assertSQLContains(t, sql, "NOT v_messages.internal OR EXISTS")

	vals := argValues(args)
	assert.Contains(t, vals, threadID.String(), "thread_id arg must be present")
	assert.Contains(t, vals, callerID.String(), "callerID arg must be present")
}
