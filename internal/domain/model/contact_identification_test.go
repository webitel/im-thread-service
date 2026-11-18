package model

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMergeCandidates(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)

	flowPhone := MatchReason{Type: CriterionPhone, Trust: TrustUnverified, Origin: OriginFlow}
	channelPhone := MatchReason{Type: CriterionPhone, Trust: TrustVerified, Origin: OriginChannel}
	messenger := MatchReason{Type: CriterionMessengerID, Trust: TrustVerified, Origin: OriginChannel}

	existing := ContactCandidates{
		{ThreadID: threadID, DomainID: 1, CRMContactID: "2", Trust: TrustUnverified, Reasons: []MatchReason{flowPhone}, CreatedAt: t0, UpdatedAt: t0},
	}

	tests := []struct {
		name        string
		matches     []ContactMatch
		wantOrder   []string
		wantChanged []string
		wantTrust   map[string]IdentificationTrust
		wantReasons map[string]int
	}{
		{
			name:        "same reason again changes nothing",
			matches:     []ContactMatch{{CRMContactID: "2", Reason: flowPhone}},
			wantOrder:   []string{"2"},
			wantTrust:   map[string]IdentificationTrust{"2": TrustUnverified},
			wantReasons: map[string]int{"2": 1},
		},
		{
			name:        "new verified contact is listed first",
			matches:     []ContactMatch{{CRMContactID: "3", Reason: messenger}},
			wantOrder:   []string{"3", "2"},
			wantChanged: []string{"3"},
			wantTrust:   map[string]IdentificationTrust{"2": TrustUnverified, "3": TrustVerified},
			wantReasons: map[string]int{"2": 1, "3": 1},
		},
		{
			name:        "known contact gains a verified reason and trust becomes the max",
			matches:     []ContactMatch{{CRMContactID: "2", Reason: channelPhone}},
			wantOrder:   []string{"2"},
			wantChanged: []string{"2"},
			wantTrust:   map[string]IdentificationTrust{"2": TrustVerified},
			wantReasons: map[string]int{"2": 2},
		},
		{
			name:        "blank ids are ignored and duplicates in one run collapse",
			matches:     []ContactMatch{{CRMContactID: " ", Reason: flowPhone}, {CRMContactID: "4", Reason: flowPhone}, {CRMContactID: "4", Reason: flowPhone}},
			wantOrder:   []string{"2", "4"},
			wantChanged: []string{"4"},
			wantTrust:   map[string]IdentificationTrust{"2": TrustUnverified, "4": TrustUnverified},
			wantReasons: map[string]int{"2": 1, "4": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			all, changed := MergeCandidates(existing, tt.matches, nil, false, threadID, 1, t1)

			var order, changedIDs []string
			for _, c := range all {
				order = append(order, c.CRMContactID)
				require.Equal(t, tt.wantTrust[c.CRMContactID], c.Trust, c.CRMContactID)
				require.Len(t, c.Reasons, tt.wantReasons[c.CRMContactID], c.CRMContactID)
			}

			for _, c := range changed {
				changedIDs = append(changedIDs, c.CRMContactID)
				require.Equal(t, t1, c.UpdatedAt)
				require.Equal(t, threadID, c.ThreadID)
			}

			require.Equal(t, tt.wantOrder, order)
			require.Equal(t, tt.wantChanged, changedIDs)
			require.Equal(t, TrustUnverified, existing[0].Trust, "input is not mutated")
			require.Len(t, existing[0].Reasons, 1, "input is not mutated")
		})
	}
}

func TestMergeCandidates_CapsNewContacts(t *testing.T) {
	t.Parallel()

	matches := make([]ContactMatch, 0, MaxCandidatesPerThread+5)
	for range MaxCandidatesPerThread + 5 {
		matches = append(matches, ContactMatch{CRMContactID: uuid.NewString(), Reason: MatchReason{Type: CriterionPhone, Trust: TrustUnverified, Origin: OriginFlow}})
	}

	all, changed := MergeCandidates(nil, matches, nil, false, uuid.New(), 1, time.Now())
	require.Len(t, all, MaxCandidatesPerThread)
	require.Len(t, changed, MaxCandidatesPerThread)

	unverified := MatchReason{Type: CriterionEmail, Trust: TrustUnverified, Origin: OriginFlow}
	verified := MatchReason{Type: CriterionMessengerID, Trust: TrustVerified, Origin: OriginChannel}
	all, changed = MergeCandidates(all, []ContactMatch{
		{CRMContactID: all[0].CRMContactID, Reason: verified},
		{CRMContactID: "new-unverified", Reason: unverified},
		{CRMContactID: "new-verified", Reason: verified},
	}, nil, false, uuid.New(), 1, time.Now())
	require.Len(t, all, MaxCandidatesPerThread+1, "a full list takes no new unverified contact, but a verified one")
	require.Len(t, changed, 2, "known contacts still gain reasons")
	require.Equal(t, TrustVerified, all[0].Trust, "verified are listed first")

	flood := make([]ContactMatch, 0, 2*MaxCandidatesPerThread)
	for range 2 * MaxCandidatesPerThread {
		flood = append(flood, ContactMatch{CRMContactID: uuid.NewString(), Reason: verified})
	}

	all, _ = MergeCandidates(all, flood, nil, false, uuid.New(), 1, time.Now())
	require.Len(t, all, 2*MaxCandidatesPerThread, "verified contacts are bounded too")
}

func TestMergeCandidates_StaleMessengerLink(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	now := time.Now()
	messenger := MatchReason{Type: CriterionMessengerID, Trust: TrustVerified, Origin: OriginChannel}
	flowEmail := MatchReason{Type: CriterionEmail, Trust: TrustUnverified, Origin: OriginFlow}

	existing := ContactCandidates{
		{ThreadID: threadID, DomainID: 1, CRMContactID: "old", Trust: TrustVerified, Reasons: []MatchReason{messenger, flowEmail}},
		{ThreadID: threadID, DomainID: 1, CRMContactID: "new", Trust: TrustVerified, Reasons: []MatchReason{messenger}},
	}

	tests := []struct {
		name        string
		owners      []string
		known       bool
		wantTrust   map[string]IdentificationTrust
		wantChanged []string
	}{
		{
			name:      "owners unknown: nothing is downgraded",
			owners:    nil,
			known:     false,
			wantTrust: map[string]IdentificationTrust{"old": TrustVerified, "new": TrustVerified},
		},
		{
			name:        "link moved to another contact: the old owner becomes a hint",
			owners:      []string{"new"},
			known:       true,
			wantTrust:   map[string]IdentificationTrust{"old": TrustUnverified, "new": TrustVerified},
			wantChanged: []string{"old"},
		},
		{
			name:        "link removed: every messenger reason is stale",
			owners:      nil,
			known:       true,
			wantTrust:   map[string]IdentificationTrust{"old": TrustUnverified, "new": TrustUnverified},
			wantChanged: []string{"old", "new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			all, changed := MergeCandidates(existing, nil, tt.owners, tt.known, threadID, 1, now)

			for _, c := range all {
				require.Equal(t, tt.wantTrust[c.CRMContactID], c.Trust, c.CRMContactID)
			}

			var changedIDs []string
			for _, c := range changed {
				changedIDs = append(changedIDs, c.CRMContactID)
			}

			require.ElementsMatch(t, tt.wantChanged, changedIDs)
			require.Equal(t, TrustVerified, existing[0].Trust, "input is not mutated")
			require.Equal(t, TrustVerified, existing[0].Reasons[0].Trust, "input is not mutated")
		})
	}
}

// The old owner of a moved link keeps a verified reason of another kind: it
// stays verified, and only the stale messenger-id reason is downgraded.
func TestMergeCandidates_StaleLinkKeepsOtherVerifiedReasons(t *testing.T) {
	t.Parallel()

	messenger := MatchReason{Type: CriterionMessengerID, Trust: TrustVerified, Origin: OriginChannel}
	channelPhone := MatchReason{Type: CriterionPhone, Trust: TrustVerified, Origin: OriginChannel}
	existing := ContactCandidates{{CRMContactID: "old", Trust: TrustVerified, Reasons: []MatchReason{messenger, channelPhone}}}

	all, changed := MergeCandidates(existing, nil, []string{"new"}, true, uuid.New(), 1, time.Now())

	require.Len(t, changed, 1)
	require.Equal(t, TrustVerified, all[0].Trust)
	require.Equal(t, []MatchReason{{Type: CriterionMessengerID, Trust: TrustUnverified, Origin: OriginChannel}, channelPhone}, all[0].Reasons)
}

// The same match seen again with stronger trust (the stored number was fixed
// to the very same digits) upgrades the reason and the candidate.
func TestMergeCandidates_SameReasonGainsTrust(t *testing.T) {
	t.Parallel()

	tail := MatchReason{Type: CriterionPhone, Trust: TrustUnverified, Origin: OriginChannel}
	exact := MatchReason{Type: CriterionPhone, Trust: TrustVerified, Origin: OriginChannel}
	existing := ContactCandidates{{CRMContactID: "1", Trust: TrustUnverified, Reasons: []MatchReason{tail}}}

	all, changed := MergeCandidates(existing, []ContactMatch{{CRMContactID: "1", Reason: exact}}, nil, false, uuid.New(), 1, time.Now())
	require.Len(t, changed, 1)
	require.Equal(t, TrustVerified, all[0].Trust)
	require.Equal(t, []MatchReason{exact}, all[0].Reasons, "upgraded in place, not added twice")

	_, changed = MergeCandidates(all, []ContactMatch{{CRMContactID: "1", Reason: tail}}, nil, false, uuid.New(), 1, time.Now())
	require.Empty(t, changed, "a weaker repeat changes nothing")
}

func TestContactCandidates_SortForDisplay(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	cs := ContactCandidates{
		{CRMContactID: "9", Trust: TrustUnverified, CreatedAt: at},
		{CRMContactID: "3", Trust: TrustUnverified, CreatedAt: at.Add(time.Second)},
		{CRMContactID: "2", Trust: TrustUnverified, CreatedAt: at},
		{CRMContactID: "5", Trust: TrustVerified, CreatedAt: at.Add(time.Hour)},
	}

	cs.SortForDisplay()

	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.CRMContactID)
	}

	require.Equal(t, []string{"5", "2", "9", "3"}, ids, "verified first, then oldest, then by id")
}

func TestNewContactDuplicate(t *testing.T) {
	t.Parallel()

	require.Nil(t, NewContactDuplicate(uuid.New(), 1, ContactCandidates{{CRMContactID: "1", Trust: TrustVerified}}))

	dup := NewContactDuplicate(uuid.New(), 1, ContactCandidates{
		{CRMContactID: "9", Trust: TrustVerified, Reasons: []MatchReason{{Type: CriterionPhone}}},
		{CRMContactID: "10", Trust: TrustVerified},
	})
	require.Equal(t, []string{"10", "9"}, dup.ContactIDs, "sorted so a set is stored once")
	require.Len(t, dup.Reasons["9"], 1)
}

func TestCRMContactBinding_IsManual(t *testing.T) {
	t.Parallel()

	require.False(t, CRMContactBinding{}.IsManual())
	require.False(t, CRMContactBinding{CRMContactID: "1", Source: CRMContactSourceAuto}.IsManual())
	require.True(t, CRMContactBinding{CRMContactID: "1"}.IsManual(), "legacy binding was set by an operator")
	require.True(t, CRMContactBinding{CRMContactID: "1", Source: CRMContactSourceSearched}.IsManual())
	require.True(t, CRMContactBinding{Source: CRMContactSourceSelected}.IsManual(), "an operator's \"no contact\"")
	require.True(t, CRMContactBinding{Source: CRMContactSourceAuto}.IsManual(),
		"an automatic contact cleared by an instance that does not track the source")
}

func TestPhoneMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		stored, wanted string
		lastN          int
		want           bool
	}{
		{"+380 (00) 000-00-01", "380000000001", 0, true},
		{"0000000001", "380000000001", 0, true},
		{"380000000002", "380000000001", 0, false},
		{"0001", "380000000001", 0, false},
		{"0000001", "380000000001", 0, true},
		{"000001", "380000000001", 0, false},
		{"12345", "12345", 0, true},
		{"380000000001", "000001", 6, true},
		{"380000123499", "1234", 4, false},
		{"12", "0012", 4, false},
		{"", "380000000001", 0, false},
		{"380000000001", "", 0, false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, PhoneMatches(tt.stored, tt.wanted, tt.lastN), "%q vs %q last %d", tt.stored, tt.wanted, tt.lastN)
	}
}

func TestEmailMatches(t *testing.T) {
	t.Parallel()

	require.True(t, EmailMatches("QA@Example.com", " qa@example.com "))
	require.False(t, EmailMatches("xqa@example.com", "qa@example.com"))
	require.False(t, EmailMatches("", ""))
}

func TestMasking(t *testing.T) {
	t.Parallel()

	require.Equal(t, "**********77", MaskPhone("+380000000077"))
	require.Equal(t, "**", MaskPhone("12"))
	require.Equal(t, "q***@e***.com", MaskEmail("qa-ws-166@example.com"))
	require.Equal(t, "q***@***", MaskEmail("qa@"))
	require.Equal(t, "***", MaskEmail("no-at-sign"))
	require.Equal(t, "*****", MaskValue(CriterionAttribute, "A-123"))
	require.Equal(t, "********", MaskValue(CriterionAttribute, "a very long attribute value"))
}

func TestIsSearchableKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want bool
	}{
		{"contract_no", true},
		{"номер.договору", true},
		{"with space", true},
		{"a' or '1", true},
		{"", false},
		{"a=b", false},
		{"a*b", false},
		{"50%", false},
		{`a\b`, false},
		{"/regexp/", false},
		{strings.Repeat("k", MaxAttributeKeyLen), true},
		{strings.Repeat("k", MaxAttributeKeyLen+1), false},
		{strings.Repeat("я", MaxAttributeKeyLen), true},
		{strings.Repeat("я", MaxAttributeKeyLen+1), false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, IsSearchableKey(tt.key), "%q", tt.key)
	}
}
