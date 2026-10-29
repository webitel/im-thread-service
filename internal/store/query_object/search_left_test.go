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

func TestSearchLeftQuery_Defaults_IncludeSourceAndQueue(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "as source")
	assertSQLContains(t, sql, "as queue")
	assertSQLContains(t, sql, "im_thread.source_ref src on src.gate_id = srcv.gate_id")
	assertSQLContains(t, sql, "qr.queue_id = lm.queue_id and qr.deleted = false")
}

func TestSearchLeftQuery_CTESelectsQueueID(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "deleted_at AS left_at, queue_id")
}

func TestSearchLeftQuery_Source_JoinsSourceRef(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id", "source"}).
		ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "from im_thread.thread_dialog sd")
	assertSQLContains(t, sql, "jsonb_build_object('id', srcv.gate_id, 'name', coalesce(src.name, '')")
	assert.NotContains(t, sql, "queue_ref")
}

func TestSearchLeftQuery_Queue_FromMembershipPeriod(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id", "queue"}).
		ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "LEFT JOIN im_thread.queue_ref qr on qr.domain_id = t.domain_id and qr.queue_id = lm.queue_id and qr.deleted = false")
	assertSQLContains(t, sql, "case when qr.queue_id is null then null")
	assert.NotContains(t, sql, "'name', '')")
	assert.NotContains(t, sql, "source_ref")
}

func TestSearchLeftQuery_WithoutSourceAndQueue_NoRefJoins(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id", "subject"}).
		ToSQL()
	require.NoError(t, err)

	assert.NotContains(t, sql, "source_ref")
	assert.NotContains(t, sql, "queue_ref")
}

func TestSearchLeftQuery_SortQueueNameNullsLast(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id"}).
		WithSort("-queue_name").
		ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "ORDER BY qr.name DESC NULLS LAST")
	assertSQLContains(t, sql, "im_thread.queue_ref qr")
	assert.NotContains(t, sql, "lm.left_at DESC")
}

func TestSearchLeftQuery_SortSourceNameNullsLast(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id"}).
		WithSort("+source_name").
		ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "ORDER BY src.name ASC NULLS LAST")
}

func TestSearchLeftQuery_SortOnlyFields_NotSelected(t *testing.T) {
	sql, _, err := NewSearchLeftQueryObject(uuid.New()).
		WithFields([]string{"id", "source_name", "queue_name"}).
		ToSQL()
	require.NoError(t, err)

	assert.NotContains(t, sql, "as source_name")
	assert.NotContains(t, sql, "as queue_name")
	assertSQLContains(t, sql, "lm.id as id")
}
