package model

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IdentificationTrust is how reliable a match is. Higher is stronger, so the
// trust of a candidate is the maximum over its reasons.
type IdentificationTrust int32

const (
	TrustUnspecified IdentificationTrust = 0
	TrustUnverified  IdentificationTrust = 1
	TrustVerified    IdentificationTrust = 2
)

// CriterionType is what an identification value is matched against.
type CriterionType int32

const (
	CriterionUnspecified CriterionType = 0
	CriterionPhone       CriterionType = 1
	CriterionEmail       CriterionType = 2
	CriterionAttribute   CriterionType = 3
	CriterionMessengerID CriterionType = 4
)

// IdentificationOrigin is the identification step that produced a match.
type IdentificationOrigin int32

const (
	OriginUnspecified IdentificationOrigin = 0
	OriginChannel     IdentificationOrigin = 1
	OriginFlow        IdentificationOrigin = 2
)

// PhoneScope limits which CRM contact phones are matched.
type PhoneScope int32

const (
	PhoneScopeAny         PhoneScope = 0
	PhoneScopePrimaryOnly PhoneScope = 1
)

// CRMContactSource is how the thread's CRM contact was bound.
type CRMContactSource int32

const (
	CRMContactSourceUnspecified CRMContactSource = 0
	CRMContactSourceAuto        CRMContactSource = 1
	CRMContactSourceSelected    CRMContactSource = 2
	CRMContactSourceCreated     CRMContactSource = 3
	CRMContactSourceSearched    CRMContactSource = 4
)

// IsManual reports whether an operator chose the contact. A manual binding is
// never replaced by identification.
func (s CRMContactSource) IsManual() bool {
	return s == CRMContactSourceSelected || s == CRMContactSourceCreated || s == CRMContactSourceSearched
}

// CRMContactBinding is the CRM contact bound to a thread.
type CRMContactBinding struct {
	CRMContactID string
	Source       CRMContactSource
}

// IsManual reports whether the binding was made by an operator. A binding made
// before the source was tracked counts as manual: only operators could set it.
// So does an automatic binding whose contact is gone: identification never
// clears a contact, so an operator cleared it through an instance that does
// not track the source (a rolling deploy), and it must not be bound again.
func (b CRMContactBinding) IsManual() bool {
	switch {
	case b.Source.IsManual():
		return true
	case b.Source == CRMContactSourceUnspecified:
		return b.CRMContactID != ""
	case b.Source == CRMContactSourceAuto:
		return b.CRMContactID == ""
	default:
		return false
	}
}

// IdentificationCriterion is one value to match, as sent by a Flow schema.
type IdentificationCriterion struct {
	Type  CriterionType
	Value string
	// Key names the CRM contact variable for CriterionAttribute.
	Key string
}

// IdentifyContactCommand runs identification for a thread.
type IdentifyContactCommand struct {
	ThreadID        uuid.UUID
	DomainID        int
	Criteria        []IdentificationCriterion
	PhoneLastDigits int
	PhoneScope      PhoneScope
}

// IdentifyContactResult is the summary returned to a Flow schema.
type IdentifyContactResult struct {
	FoundCount int
	// CRMContactID is set only when the thread has exactly one candidate, it
	// is verified and it is the contact bound to the thread.
	CRMContactID string
	Trust        IdentificationTrust
}

// MatchReason is why a CRM contact became a candidate. It never carries the
// matched value: only its kind, trust and origin.
type MatchReason struct {
	Type   CriterionType        `json:"type"`
	Trust  IdentificationTrust  `json:"trust"`
	Origin IdentificationOrigin `json:"origin"`
	Key    string               `json:"key,omitempty"`
}

func (r MatchReason) sameMatch(o MatchReason) bool {
	return r.Type == o.Type && r.Origin == o.Origin && r.Key == o.Key
}

// ContactCandidate is a CRM contact identified for a thread.
type ContactCandidate struct {
	ThreadID     uuid.UUID
	DomainID     int
	CRMContactID string
	Trust        IdentificationTrust
	Reasons      []MatchReason
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ContactCandidates is the thread's candidate list.
type ContactCandidates []*ContactCandidate

// Verified returns the candidates with a verified reason.
func (cs ContactCandidates) Verified() ContactCandidates {
	var out ContactCandidates

	for _, c := range cs {
		if c.Trust == TrustVerified {
			out = append(out, c)
		}
	}

	return out
}

// MaxTrust is the highest trust in the list; TrustUnspecified when empty.
func (cs ContactCandidates) MaxTrust() IdentificationTrust {
	trust := TrustUnspecified
	for _, c := range cs {
		trust = max(trust, c.Trust)
	}

	return trust
}

// SortForDisplay orders verified candidates first, then by first match time
// and id, so the client never shows a doubtful candidate before a verified one.
func (cs ContactCandidates) SortForDisplay() {
	slices.SortStableFunc(cs, func(a, b *ContactCandidate) int {
		return cmp.Or(
			cmp.Compare(b.Trust, a.Trust),
			a.CreatedAt.Compare(b.CreatedAt),
			cmp.Compare(a.CRMContactID, b.CRMContactID),
		)
	})
}

// ContactMatch is one CRM contact found by one search.
type ContactMatch struct {
	CRMContactID string
	Reason       MatchReason
}

// MaxCandidatesPerThread bounds a thread's candidate list: new unverified
// contacts past it are dropped, known ones still gain reasons. Verified
// contacts are still added, up to twice the bound, so a list full of possible
// matches cannot hide the one that may be bound.
const MaxCandidatesPerThread = 100

// MergeCandidates appends matches to the existing candidates (never replacing
// them): new contacts are added, known ones gain the new reasons and their
// trust becomes the maximum. It returns the full list sorted for display and
// the candidates that were added or changed, which the caller persists.
//
// linkOwners, when linkOwnersKnown, are the CRM contacts the thread's IM
// contact is linked to right now: a messenger-id reason of any other candidate
// is stale (the link was moved) and is downgraded to unverified, so the old
// owner stays a hint but is no longer bound or reported as a duplicate.
func MergeCandidates(existing ContactCandidates, matches []ContactMatch, linkOwners []string, linkOwnersKnown bool, threadID uuid.UUID, domainID int, now time.Time) (all, changed ContactCandidates) {
	byID := make(map[string]*ContactCandidate, len(existing)+len(matches))
	dirty := make(map[string]bool)

	for _, c := range existing {
		cp := *c
		cp.Reasons = slices.Clone(c.Reasons)
		byID[c.CRMContactID] = &cp
		all = append(all, &cp)

		if linkOwnersKnown && !slices.Contains(linkOwners, cp.CRMContactID) && cp.dropStaleLink() {
			dirty[cp.CRMContactID] = true
		}
	}

	for _, m := range matches {
		id := strings.TrimSpace(m.CRMContactID)
		if id == "" {
			continue
		}

		c, ok := byID[id]
		if !ok {
			if limit := MaxCandidatesPerThread; len(all) >= 2*limit || (len(all) >= limit && m.Reason.Trust != TrustVerified) {
				continue
			}

			c = &ContactCandidate{ThreadID: threadID, DomainID: domainID, CRMContactID: id, CreatedAt: now}
			byID[id] = c
			all = append(all, c)
			dirty[id] = true
		}

		if c.addReason(m.Reason) {
			dirty[id] = true
		}
	}

	for _, c := range all {
		if dirty[c.CRMContactID] {
			c.UpdatedAt = now
			changed = append(changed, c)
		}
	}

	all.SortForDisplay()

	return all, changed
}

// addReason records r, keeping the stronger trust of an equal reason, and
// reports whether the candidate changed.
func (c *ContactCandidate) addReason(r MatchReason) bool {
	for i, have := range c.Reasons {
		if !have.sameMatch(r) {
			continue
		}

		if r.Trust <= have.Trust {
			return false
		}

		c.Reasons[i].Trust = r.Trust
		c.Trust = max(c.Trust, r.Trust)

		return true
	}

	c.Reasons = append(c.Reasons, r)
	c.Trust = max(c.Trust, r.Trust)

	return true
}

// dropStaleLink downgrades a verified messenger-id reason and recomputes the
// candidate's trust; it reports whether anything changed.
func (c *ContactCandidate) dropStaleLink() bool {
	changed := false

	for i, r := range c.Reasons {
		if r.Type == CriterionMessengerID && r.Origin == OriginChannel && r.Trust == TrustVerified {
			c.Reasons[i].Trust = TrustUnverified
			changed = true
		}
	}

	if !changed {
		return false
	}

	c.Trust = TrustUnspecified
	for _, r := range c.Reasons {
		c.Trust = max(c.Trust, r.Trust)
	}

	return true
}

// ContactDuplicate records two or more verified candidates of one thread.
// Contacts are never merged automatically.
type ContactDuplicate struct {
	ThreadID uuid.UUID
	DomainID int
	// ContactIDs are sorted so the same set is stored once.
	ContactIDs []string
	Reasons    map[string][]MatchReason
}

// NewContactDuplicate builds the duplicate record of the verified candidates;
// nil when fewer than two are verified.
func NewContactDuplicate(threadID uuid.UUID, domainID int, verified ContactCandidates) *ContactDuplicate {
	if len(verified) < 2 {
		return nil
	}

	dup := &ContactDuplicate{
		ThreadID: threadID,
		DomainID: domainID,
		Reasons:  make(map[string][]MatchReason, len(verified)),
	}

	for _, c := range verified {
		dup.ContactIDs = append(dup.ContactIDs, c.CRMContactID)
		dup.Reasons[c.CRMContactID] = slices.Clone(c.Reasons)
	}

	slices.Sort(dup.ContactIDs)

	return dup
}

// CRMContact is the part of a Webitel CRM contact identification compares.
type CRMContact struct {
	ID           string
	Phones       []CRMPhone
	Emails       []string
	Variables    map[string]string
	IMContactIDs []string
}

// CRMPhone is a CRM contact phone number.
type CRMPhone struct {
	Number  string
	Primary bool
}

// CRMSearchField is the part of a CRM contact a search term is matched in.
type CRMSearchField int

const (
	// CRMSearchIMContact matches the id of an IM contact linked to the contact, exactly.
	CRMSearchIMContact CRMSearchField = iota + 1
	// CRMSearchPhones matches a substring of the contact's phone numbers.
	CRMSearchPhones
	// CRMSearchEmails matches a substring of the contact's emails.
	CRMSearchEmails
	// CRMSearchVariables matches "key=value" in the contact's variables.
	CRMSearchVariables
)

// CRMContactQuery is one page of a CRM contact search: Term matched in Field.
type CRMContactQuery struct {
	DomainID int
	Field    CRMSearchField
	Term     string
	Size     int32
	Page     int32
}

// IMContactLink links the thread's IM contact to a CRM contact (enrichment).
type IMContactLink struct {
	DomainID     int
	CRMContactID string
	IMContactID  uuid.UUID
	// Protocol is the provider type of the thread's gate, e.g. "whatsapp".
	Protocol string
	// Via is the gate id the IM contact writes through.
	Via string
}

// ErrIMContactLinked is returned when the CRM refuses a link because a CRM
// contact already holds the IM contact.
var ErrIMContactLinked = errors.New("im contact is already linked to a crm contact")

// ErrChannelIdentityNotFound is returned when the thread's IM contact does not exist.
var ErrChannelIdentityNotFound = errors.New("channel identity not found")

// ChannelIdentity is the IM contact of the thread's external member: the
// channel identifiers searched on every identification run.
type ChannelIdentity struct {
	IMContactID uuid.UUID
	Issuer      string
	Type        string
	// Phone is set by providers that identify users by number (WhatsApp).
	Phone string
}
