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

// stripSpaces removes all whitespace so structural SQL assertions are insensitive to the exact
// spacing produced by CompactSQL (which drops spaces around punctuation).
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

// argValues renders args as strings so value assertions don't depend on whether squirrel stored a
// uuid.UUID or its driver.Valuer string form.
func argValues(args []any) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = fmt.Sprintf("%v", a)
	}

	return out
}

func TestThreadQuery_WithParticipantsFilter_SingleParticipant(t *testing.T) {
	selfID := uuid.New()

	query := NewThreadQueryObject().
		WithParticipantsFilter(selfID, []int{7}, dto.ContactIdentity{Sub: "sub-1", Iss: "telegram"})

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	// CTE resolves (iss, sub) -> contact id, scoped by domain.
	assertSQLContains(t, sql, "WITH participant_contacts AS (")
	assertSQLContains(t, sql, "SELECT c.id FROM im_contact.contact c")
	assertSQLContains(t, sql, "c.domain_id IN (")
	assertSQLContains(t, sql, "(c.issuer_id, c.subject_id) IN ((")

	// Self scope + membership over the CTE.
	assertSQLContains(t, sql, "td.member_id =")
	assertSQLContains(t, sql, "td.deleted_at IS NULL")
	assertSQLContains(t, sql, "SELECT id FROM participant_contacts")
	assertSQLContains(t, sql, "HAVING COUNT(DISTINCT td2.member_id) = (SELECT COUNT(*) FROM participant_contacts)")

	// Prefix args lead the slice (domain, iss, sub), then the self-scope arg.
	require.Len(t, args, 4)
	assert.Equal(t, []string{"7", "telegram", "sub-1", selfID.String()}, argValues(args))
}

func TestThreadQuery_WithParticipantsFilter_MultipleParticipants(t *testing.T) {
	selfID := uuid.New()

	query := NewThreadQueryObject().
		WithParticipantsFilter(selfID, []int{7},
			dto.ContactIdentity{Sub: "sub-1", Iss: "telegram"},
			dto.ContactIdentity{Sub: "sub-2", Iss: "whatsapp"},
		)

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	// Two (iss, sub) tuples in the CTE.
	assertSQLContains(t, sql, "(c.issuer_id, c.subject_id) IN ((")
	// The HAVING count enforces that every participant is a member (AND semantics).
	assertSQLContains(t, sql, "HAVING COUNT(DISTINCT td2.member_id) = (SELECT COUNT(*) FROM participant_contacts)")

	// domain, iss1, sub1, iss2, sub2, self
	require.Len(t, args, 6)
	assert.Equal(t, []string{"7", "telegram", "sub-1", "whatsapp", "sub-2", selfID.String()}, argValues(args))
}

func TestThreadQuery_WithParticipantsFilter_NoDomain_OmitsDomainClause(t *testing.T) {
	selfID := uuid.New()

	query := NewThreadQueryObject().
		WithParticipantsFilter(selfID, nil, dto.ContactIdentity{Sub: "sub-1", Iss: "telegram"})

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	// CTE still present, but without the domain scoping clause.
	assertSQLContains(t, sql, "WITH participant_contacts AS (")
	assert.NotContains(t, stripSpaces(sql), "c.domain_idIN(")
	// Self scope is still enforced (the security boundary).
	assertSQLContains(t, sql, "td.member_id =")

	// iss, sub, self
	require.Len(t, args, 3)
	assert.Equal(t, []string{"telegram", "sub-1", selfID.String()}, argValues(args))
}

func TestThreadQuery_WithParticipantsFilter_NoParticipants_NoOp(t *testing.T) {
	selfID := uuid.New()

	query := NewThreadQueryObject().
		WithParticipantsFilter(selfID, []int{7})

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	assert.NotContains(t, sql, "participant_contacts")
	assert.Empty(t, args)
}

func TestThreadQuery_WithParticipantsFilter_NilSelf_NoOp(t *testing.T) {
	query := NewThreadQueryObject().
		WithParticipantsFilter(uuid.Nil, []int{7}, dto.ContactIdentity{Sub: "sub-1", Iss: "telegram"})

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	assert.NotContains(t, sql, "participant_contacts")
	assert.Empty(t, args)
}

// TestThreadQuery_WithParticipantsFilter_ComposesWithOtherFilters verifies the CTE prefix args stay
// at the head of the arg slice even when other filters (which append WHERE args) are present — the
// Dollar placeholder renumbering depends on this ordering holding.
func TestThreadQuery_WithParticipantsFilter_ComposesWithOtherFilters(t *testing.T) {
	selfID := uuid.New()

	query := NewThreadQueryObject().
		WithSubject().
		WithDomainIDFilter(7).
		WithoutDeletedAtFilter().
		WithSort("-created_at").
		WithParticipantsFilter(selfID, []int{7}, dto.ContactIdentity{Sub: "sub-1", Iss: "telegram"})

	sql, args, err := query.ToSQL()
	require.NoError(t, err)

	assertSQLContains(t, sql, "WITH participant_contacts AS (")
	assertSQLContains(t, sql, "t.domain_id IN (") // from WithDomainIDFilter
	assertSQLContains(t, sql, "ORDER BY t.created_at DESC")

	// Prefix (CTE) args must come first regardless of when other filters were applied.
	require.GreaterOrEqual(t, len(args), 4)
	values := argValues(args)
	assert.Equal(t, "7", values[0])        // CTE domain
	assert.Equal(t, "telegram", values[1]) // iss
	assert.Equal(t, "sub-1", values[2])    // sub
	assert.Contains(t, values, selfID.String())
}
