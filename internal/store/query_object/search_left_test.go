package queryobject

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchLeftQuery_WithTagsFilter_MatchesSubsetOfThreadsTags mirrors
// TestThreadQuery_WithTagsFilter_MatchesSubsetOfThreadsTags: a thread with more tags than were
// searched for must still match, as long as it carries every searched tag.
func TestSearchLeftQuery_WithTagsFilter_MatchesSubsetOfThreadsTags(t *testing.T) {
	memberID := uuid.New()

	query := NewSearchLeftQueryObject(memberID).WithTagsFilter(memberID, "work")

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "t.id IN (")
	assertSQLContains(t, sql, "SELECT thread_id FROM im_thread.thread_tag")
	assertSQLContains(t, sql, "WHERE contact_id =")
	assertSQLContains(t, sql, "AND tag = ANY(")
	assertSQLContains(t, sql, "GROUP BY thread_id")
	assertSQLContains(t, sql, "HAVING COUNT(DISTINCT tag) =")

	// args: [0] is the membership_periods CTE's memberID, appended after that are the tags filter's own.
	require.GreaterOrEqual(t, len(args), 4)
	values := argValues(args)
	assert.Contains(t, values, "[work]")
	assert.Equal(t, "1", values[len(values)-1]) // HAVING compares against len(searched tags), not the thread's tag count
}

func TestSearchLeftQuery_WithTagsFilter_NilMember_NoOp(t *testing.T) {
	query := NewSearchLeftQueryObject(uuid.New()).WithTagsFilter(uuid.Nil, "work")

	sql, _, err := query.ToSQL()
	require.NoError(t, err)
	assert.NotContains(t, sql, "thread_tag")
}

func TestSearchLeftQuery_WithTagsFilter_NoTags_NoOp(t *testing.T) {
	query := NewSearchLeftQueryObject(uuid.New()).WithTagsFilter(uuid.New())

	sql, _, err := query.ToSQL()
	require.NoError(t, err)
	assert.NotContains(t, sql, "thread_tag")
}
