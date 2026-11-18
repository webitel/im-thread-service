package service

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// fakeContactCandidateStore keeps candidates, duplicates and the channel mark in memory.
type fakeContactCandidateStore struct {
	rows       map[string]*model.ContactCandidate
	duplicates [][]string
	checked    bool
	checkedAt  time.Time
	upserts    int
}

func (f *fakeContactCandidateStore) List(context.Context, uuid.UUID, int) (model.ContactCandidates, error) {
	out := make(model.ContactCandidates, 0, len(f.rows))
	for _, c := range f.rows {
		cp := *c
		cp.Reasons = slices.Clone(c.Reasons)
		out = append(out, &cp)
	}

	out.SortForDisplay()

	return out, nil
}

func (f *fakeContactCandidateStore) Upsert(_ context.Context, cs model.ContactCandidates) error {
	if f.rows == nil {
		f.rows = map[string]*model.ContactCandidate{}
	}

	for _, c := range cs {
		cp := *c
		f.rows[c.CRMContactID] = &cp
		f.upserts++
	}

	return nil
}

func (f *fakeContactCandidateStore) SaveDuplicate(_ context.Context, dup *model.ContactDuplicate) error {
	if dup == nil {
		return nil
	}

	for _, d := range f.duplicates {
		if slices.Equal(d, dup.ContactIDs) {
			return nil
		}
	}

	f.duplicates = append(f.duplicates, dup.ContactIDs)

	return nil
}

func (f *fakeContactCandidateStore) ChannelCheckedAt(context.Context, uuid.UUID) (time.Time, bool, error) {
	return f.checkedAt, f.checked, nil
}

func (f *fakeContactCandidateStore) MarkChannelChecked(_ context.Context, _ uuid.UUID, _ int, observedAt time.Time) error {
	f.checked = true
	if observedAt.After(f.checkedAt) {
		f.checkedAt = observedAt
	}

	return nil
}

// fakeCRM answers searches per field, page by page, and records queries and links.
type fakeCRM struct {
	byField map[model.CRMSearchField][]*model.CRMContact
	err     error
	// errByField fails the searches of one field only.
	errByField map[model.CRMSearchField]error
	queries    []model.CRMContactQuery
	links      []model.IMContactLink
	linkErr    error
	// ownersLater, when set, is what later IM contact searches return: someone
	// linked the IM contact between identification and enrichment.
	ownersLater []*model.CRMContact
	imSearches  int
}

func (f *fakeCRM) SearchContacts(_ context.Context, q model.CRMContactQuery) ([]*model.CRMContact, bool, error) {
	f.queries = append(f.queries, q)

	if f.err != nil {
		return nil, false, f.err
	}

	rows := f.byField[q.Field]

	if q.Field == model.CRMSearchIMContact {
		f.imSearches++
		if f.ownersLater != nil && f.imSearches > 1 {
			rows = f.ownersLater
		}

		// The CRM matches the IM contact id exactly.
		rows = slices.DeleteFunc(slices.Clone(rows), func(c *model.CRMContact) bool { return !slices.Contains(c.IMContactIDs, q.Term) })
	}

	if err := f.errByField[q.Field]; err != nil {
		return nil, false, err
	}

	from := min(int(q.Page-1)*int(q.Size), len(rows))
	to := min(from+int(q.Size), len(rows))

	return rows[from:to], to < len(rows), nil
}

func (f *fakeCRM) LinkIMContact(_ context.Context, link model.IMContactLink) error {
	f.links = append(f.links, link)

	// The CRM now knows the owner: later IM contact searches find it.
	if f.linkErr == nil && f.byField != nil {
		f.byField[model.CRMSearchIMContact] = append(f.byField[model.CRMSearchIMContact], &model.CRMContact{ID: link.CRMContactID, IMContactIDs: []string{link.IMContactID.String()}})
	}

	return f.linkErr
}

type fakeChannels struct {
	identity *model.ChannelIdentity
}

func (f *fakeChannels) ChannelIdentity(_ context.Context, id uuid.UUID, _ int) (*model.ChannelIdentity, error) {
	if f.identity == nil {
		return nil, model.ErrChannelIdentityNotFound
	}

	cp := *f.identity
	cp.IMContactID = id

	return &cp, nil
}

type identificationFixture struct {
	svc        *ContactIdentificationService
	threads    *fakeThreadStore
	candidates *fakeContactCandidateStore
	outbox     *fakeOutboxStore
	crm        *fakeCRM
	client     uuid.UUID
	via        string
	threadID   uuid.UUID
	logs       *bytes.Buffer
}

func newIdentificationFixture(t *testing.T, identity *model.ChannelIdentity, crm *fakeCRM) *identificationFixture {
	t.Helper()

	via := uuid.NewString()
	f := &identificationFixture{
		threads:    &fakeThreadStore{},
		candidates: &fakeContactCandidateStore{},
		outbox:     &fakeOutboxStore{},
		crm:        crm,
		client:     uuid.New(),
		via:        via,
		threadID:   uuid.New(),
		logs:       &bytes.Buffer{},
	}

	dialogs := &fakeThreadDialogStore{quickViewResult: []*model.ThreadDialog{
		{ContactID: uuid.New(), ThreadRole: model.RoleMember},
		{ContactID: uuid.New(), ThreadRole: model.RoleMember, IsBot: true, Via: &via},
		{ContactID: f.client, ThreadRole: model.RoleOwner, Via: &via},
	}}

	f.svc = NewContactIdentificationService(fakeUnitOfWork{
		threadStore:       f.threads,
		threadDialogStore: dialogs,
		outboxStore:       f.outbox,
		candidateStore:    f.candidates,
	}, crm, &fakeChannels{identity: identity}, slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

	tick := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	f.svc.now = func() time.Time {
		tick = tick.Add(time.Second)

		return tick
	}

	return f
}

func (f *identificationFixture) identify(t *testing.T, criteria ...model.IdentificationCriterion) *model.IdentifyContactResult {
	t.Helper()

	res, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{ThreadID: f.threadID, DomainID: 1, Criteria: criteria})
	require.NoError(t, err)
	require.NoError(t, f.svc.Wait(context.Background()), "background links finish")

	return res
}

func phone(v string) model.IdentificationCriterion {
	return model.IdentificationCriterion{Type: model.CriterionPhone, Value: v}
}

func linkedTo(imContactIDs ...string) []*model.CRMContact {
	return []*model.CRMContact{{ID: "10", IMContactIDs: imContactIDs}}
}

func TestIdentify_BindingRules(t *testing.T) {
	t.Parallel()

	telegram := &model.ChannelIdentity{Type: "telegram"}
	whatsapp := &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}

	tests := []struct {
		name        string
		identity    *model.ChannelIdentity
		crm         func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact
		binding     model.CRMContactBinding
		criteria    []model.IdentificationCriterion
		wantFound   int
		wantContact string
		wantTrust   model.IdentificationTrust
		wantBound   []string
		wantLinks   int
		wantDups    int
	}{
		{
			name:     "messenger id linked to one contact: auto-bind, no re-link",
			identity: telegram,
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
			},
			wantFound: 1, wantContact: "10", wantTrust: model.TrustVerified, wantBound: []string{"10"},
		},
		{
			name:     "same contact already chosen by the operator: reported, not rebound",
			identity: telegram,
			binding:  model.CRMContactBinding{CRMContactID: "10", Source: model.CRMContactSourceSelected},
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
			},
			wantFound: 1, wantContact: "10", wantTrust: model.TrustVerified,
		},
		{
			name:     "whatsapp number of one contact: auto-bind and link the im contact",
			identity: whatsapp,
			crm: func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "+380 00 000 00 01"}}}}}
			},
			wantFound: 1, wantContact: "20", wantTrust: model.TrustVerified, wantBound: []string{"20"}, wantLinks: 1,
		},
		{
			name:     "whatsapp number stored without the country code: possible match, never bound",
			identity: whatsapp,
			crm: func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "21", Phones: []model.CRMPhone{{Number: "0000000001"}}}}}
			},
			wantFound: 1, wantTrust: model.TrustUnverified,
		},
		{
			name:     "single unverified match from flow: possible match, never bound",
			identity: telegram,
			crm: func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "30", Phones: []model.CRMPhone{{Number: "380000000002"}}}}}
			},
			criteria:  []model.IdentificationCriterion{phone("+380000000002")},
			wantFound: 1, wantTrust: model.TrustUnverified,
		},
		{
			name:     "verified A and unverified B: A bound, B stays a hint",
			identity: telegram,
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{
					model.CRMSearchIMContact: linkedTo(f.client.String()),
					model.CRMSearchPhones:    {{ID: "30", Phones: []model.CRMPhone{{Number: "380000000002"}}}},
				}
			},
			criteria:  []model.IdentificationCriterion{phone("380000000002")},
			wantFound: 2, wantTrust: model.TrustVerified, wantBound: []string{"10"},
		},
		{
			name:     "two verified contacts: operator chooses, duplicate recorded",
			identity: whatsapp,
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{
					model.CRMSearchIMContact: linkedTo(f.client.String()),
					model.CRMSearchPhones:    {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}},
				}
			},
			wantFound: 2, wantTrust: model.TrustVerified, wantDups: 1,
		},
		{
			name:     "manual binding is never replaced",
			identity: telegram,
			binding:  model.CRMContactBinding{CRMContactID: "99", Source: model.CRMContactSourceSelected},
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
			},
			// The schema learns the contact only if it is the one on the thread.
			wantFound: 1, wantContact: "", wantTrust: model.TrustVerified,
		},
		{
			name:     "operator's unverified choice is not reported to the schema",
			identity: telegram,
			binding:  model.CRMContactBinding{CRMContactID: "30", Source: model.CRMContactSourceSelected},
			crm: func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "30", Phones: []model.CRMPhone{{Number: "380000000002"}}}}}
			},
			criteria:  []model.IdentificationCriterion{phone("380000000002")},
			wantFound: 1, wantContact: "", wantTrust: model.TrustUnverified,
		},
		{
			name:     "automatic contact cleared by an older instance is not bound again",
			identity: telegram,
			binding:  model.CRMContactBinding{Source: model.CRMContactSourceAuto},
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
			},
			wantFound: 1, wantContact: "", wantTrust: model.TrustVerified,
		},
		{
			name:     "binding from before source tracking counts as manual",
			identity: telegram,
			binding:  model.CRMContactBinding{CRMContactID: "99"},
			crm: func(f *identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
			},
			wantFound: 1, wantContact: "", wantTrust: model.TrustVerified,
		},
		{
			name:     "crm row linked to another im contact is not a messenger match",
			identity: telegram,
			crm: func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact {
				return map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(uuid.NewString())}
			},
			wantFound: 0,
		},
		{
			name:      "nothing found",
			identity:  telegram,
			crm:       func(*identificationFixture) map[model.CRMSearchField][]*model.CRMContact { return nil },
			criteria:  []model.IdentificationCriterion{phone("380000000009")},
			wantFound: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{}
			f := newIdentificationFixture(t, tt.identity, crm)
			crm.byField = tt.crm(f)
			f.threads.binding = tt.binding

			res := f.identify(t, tt.criteria...)

			require.Equal(t, tt.wantFound, res.FoundCount)
			require.Equal(t, tt.wantContact, res.CRMContactID)
			require.Equal(t, tt.wantTrust, res.Trust)
			require.Equal(t, tt.wantBound, f.threads.setCRMContactCalls)
			require.Len(t, crm.links, tt.wantLinks)
			require.Len(t, f.candidates.duplicates, tt.wantDups)
			require.True(t, f.candidates.checked)

			for _, src := range f.threads.setCRMContactSources {
				require.Equal(t, model.CRMContactSourceAuto, src)
			}

			if tt.wantLinks > 0 {
				require.Equal(t, model.IMContactLink{
					DomainID: 1, CRMContactID: tt.wantBound[0], IMContactID: f.client, Protocol: "whatsapp", Via: f.via,
				}, crm.links[0])
			}

			if tt.wantFound > 0 {
				require.Len(t, f.outbox.published, 1)
				updated, ok := f.outbox.published[0].event.(*event.ThreadUpdated)
				require.True(t, ok)
				require.True(t, updated.ContactCandidatesChanged)
				require.Equal(t, f.threadID, updated.ThreadID)
				require.Equal(t, f.threads.binding.CRMContactID, updated.CRMContactID, "the thread's contact after the run")
				require.Len(t, updated.Participants, 3, "every member is told")
				require.Contains(t, updated.Participants, f.client)
			} else {
				require.Empty(t, f.outbox.published)
			}
		})
	}
}

func TestIdentify_SameRequestTwiceDoesNotDoubleApply(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchIMContact: linkedTo(f.client.String()),
		model.CRMSearchPhones:    {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}},
	}

	first := f.identify(t)
	upserts, events := f.candidates.upserts, len(f.outbox.published)
	second := f.identify(t)

	require.Equal(t, first, second)
	require.Equal(t, upserts, f.candidates.upserts, "no candidate rewritten")
	require.Equal(t, events, len(f.outbox.published), "no second event")
	require.Len(t, f.candidates.duplicates, 1, "duplicate stored once")
	require.Empty(t, f.threads.setCRMContactCalls)
}

func TestIdentify_AppendsAcrossRuns(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)

	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchEmails: {{ID: "40", Emails: []string{"qa-ws-166@example.com"}}}}

	require.Equal(t, 1, f.identify(t, model.IdentificationCriterion{Type: model.CriterionEmail, Value: "QA-WS-166@example.com"}).FoundCount)

	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchVariables: {{ID: "50", Variables: map[string]string{"contract": "A-1"}}}}
	res := f.identify(t, model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "contract", Value: "A-1"})

	require.Equal(t, 2, res.FoundCount, "second run appends, never replaces")
	require.Equal(t, model.TrustUnverified, res.Trust)
	require.Equal(t, "contract", f.candidates.rows["50"].Reasons[0].Key)
}

func TestIdentify_PostFiltersCRMMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contacts map[model.CRMSearchField][]*model.CRMContact
		cmd      model.IdentifyContactCommand
		want     int
	}{
		{
			name:     "last N digits must be the end of the number, not the middle",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "380000123499"}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("1234")}, PhoneLastDigits: 4},
			want:     0,
		},
		{
			name:     "last N digits matched at the end",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "+38 (000) 000-12-34"}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("0991234")}, PhoneLastDigits: 4},
			want:     1,
		},
		{
			name:     "full number differs",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "380000000001"}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("380000000002")}},
			want:     0,
		},
		{
			name:     "local format of the same number matches",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "+380000000001"}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("000 000 00 01")}},
			want:     1,
		},
		{
			name:     "primary only skips secondary phones",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "380000000001"}, {Number: "380000000002", Primary: true}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("380000000001")}, PhoneScope: model.PhoneScopePrimaryOnly},
			want:     0,
		},
		{
			name:     "primary only matches the primary phone",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "1", Phones: []model.CRMPhone{{Number: "380000000002"}, {Number: "380000000001", Primary: true}}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{phone("380000000001")}, PhoneScope: model.PhoneScopePrimaryOnly},
			want:     1,
		},
		{
			name:     "email substring is not a match",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchEmails: {{ID: "1", Emails: []string{"xqa@example.com"}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{{Type: model.CriterionEmail, Value: "qa@example.com"}}},
			want:     0,
		},
		{
			name:     "attribute of another key is not a match",
			contacts: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchVariables: {{ID: "1", Variables: map[string]string{"other": "A-1"}}}},
			cmd:      model.IdentifyContactCommand{Criteria: []model.IdentificationCriterion{{Type: model.CriterionAttribute, Key: "contract", Value: "A-1"}}},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{byField: tt.contacts}
			f := newIdentificationFixture(t, nil, crm)
			tt.cmd.ThreadID, tt.cmd.DomainID = f.threadID, 1

			res, err := f.svc.Identify(context.Background(), &tt.cmd)
			require.NoError(t, err)
			require.Equal(t, tt.want, res.FoundCount)
		})
	}
}

func TestIdentify_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  model.IdentifyContactCommand
	}{
		{name: "no thread", cmd: model.IdentifyContactCommand{DomainID: 1}},
		{name: "no domain", cmd: model.IdentifyContactCommand{ThreadID: uuid.New()}},
		{name: "last digits too large", cmd: model.IdentifyContactCommand{ThreadID: uuid.New(), DomainID: 1, PhoneLastDigits: 16}},
		{name: "last digits too small", cmd: model.IdentifyContactCommand{ThreadID: uuid.New(), DomainID: 1, PhoneLastDigits: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{}
			f := newIdentificationFixture(t, nil, crm)

			_, err := f.svc.Identify(context.Background(), &tt.cmd)
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, errors.Code(err))
			require.Empty(t, crm.queries, "no CRM call on invalid input")
		})
	}
}

// What a client typed instead of a phone or an email must not fail the
// schema's node: the criterion is skipped, the others still run.
func TestIdentify_UnusableCriteriaAreSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		criterion model.IdentificationCriterion
	}{
		{name: "blank value", criterion: phone("  ")},
		{name: "phone without digits", criterion: phone("<script>alert(1)</script>")},
		{name: "phone with too few digits", criterion: phone("' OR 1=1 --")},
		{name: "attribute without key", criterion: model.IdentificationCriterion{Type: model.CriterionAttribute, Value: "x"}},
		{name: "attribute key with a search operator", criterion: model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "a*b", Value: "x"}},
		{name: "value of operators only", criterion: model.IdentificationCriterion{Type: model.CriterionEmail, Value: "**%%"}},
		{name: "one-letter email would scan the CRM", criterion: model.IdentificationCriterion{Type: model.CriterionEmail, Value: "a"}},
		{name: "email without @", criterion: model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-ws-166"}},
		{name: "value too long", criterion: model.IdentificationCriterion{Type: model.CriterionEmail, Value: strings.Repeat("a", maxCriterionValueLen+1)}},
		{name: "messenger id is output only", criterion: model.IdentificationCriterion{Type: model.CriterionMessengerID, Value: "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchEmails: {{ID: "40", Emails: []string{"qa-ws-166@example.com"}}}}}
			f := newIdentificationFixture(t, nil, crm)

			res := f.identify(t, tt.criterion, model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-ws-166@example.com"})

			require.Equal(t, 1, res.FoundCount, "the usable criterion still runs")
			require.Len(t, crm.queries, 1, "the unusable one never reaches the CRM")
		})
	}
}

func TestIdentify_CriteriaCappedAt16(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, nil, crm)

	criteria := make([]model.IdentificationCriterion, 0, maxCriteria+4)
	for i := range maxCriteria + 4 {
		criteria = append(criteria, model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-" + strconv.Itoa(i) + "@example.com"})
	}

	f.identify(t, criteria...)
	require.Len(t, crm.queries, maxCriteria)
}

func TestIdentify_AnotherDomainsThread(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	f.threads.bindingErr = errors.NotFound("thread not found")

	_, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{ThreadID: f.threadID, DomainID: 2})
	require.Equal(t, codes.NotFound, errors.Code(err))
	require.Empty(t, crm.queries)
}

func TestIdentify_CRMFailureStoresNothing(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{err: stderrors.New("crm down")}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)

	_, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{
		ThreadID: f.threadID, DomainID: 1, Criteria: []model.IdentificationCriterion{phone("380000000001")},
	})
	require.Error(t, err)
	require.Zero(t, f.candidates.upserts)
	require.False(t, f.candidates.checked)
	require.Empty(t, f.outbox.published)
}

func TestIdentify_OneFailingCriterionKeepsTheRest(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{errByField: map[model.CRMSearchField]error{model.CRMSearchPhones: stderrors.New("crm timeout")}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchIMContact: linkedTo(f.client.String()),
		model.CRMSearchEmails:    {{ID: "40", Emails: []string{"qa-ws-166@example.com"}}},
	}

	res := f.identify(t, phone("380000000001"), model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-ws-166@example.com"})

	require.Equal(t, 2, res.FoundCount, "channel match and the email match are kept")
	require.Equal(t, []string{"10"}, f.threads.setCRMContactCalls, "the verified channel match is still bound")
}

func TestIdentify_EveryCriterionFailingIsAnError(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{errByField: map[model.CRMSearchField]error{model.CRMSearchPhones: stderrors.New("crm down"), model.CRMSearchEmails: stderrors.New("crm down")}}
	f := newIdentificationFixture(t, nil, crm)

	_, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{
		ThreadID: f.threadID, DomainID: 1,
		Criteria: []model.IdentificationCriterion{phone("380000000001"), {Type: model.CriterionEmail, Value: "qa-ws-166@example.com"}},
	})
	require.Equal(t, codes.Internal, errors.Code(err), "\"found nothing\" would mislead the schema")
	require.Zero(t, f.candidates.upserts)
}

func TestIdentify_LogsNeverCarryRawValues(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{err: stderrors.New("crm down")}
	f := newIdentificationFixture(t, nil, crm)

	_, _ = f.svc.Identify(context.Background(), &model.IdentifyContactCommand{
		ThreadID: f.threadID, DomainID: 1, Criteria: []model.IdentificationCriterion{
			phone("+380000000077"),
		},
	})

	require.NotContains(t, f.logs.String(), "380000000077")
	require.Contains(t, f.logs.String(), "77")
}

func TestIdentify_CRMSearchTermsArePlainSubstrings(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, nil, crm)

	f.identify(t,
		phone("+38 (000) 000-00-01"),
		model.IdentificationCriterion{Type: model.CriterionEmail, Value: " /qa@example.com/ "},
		model.IdentificationCriterion{Type: model.CriterionEmail, Value: "=qa*x@example.com"},
		model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "contract", Value: "%A-1%"},
		model.IdentificationCriterion{Type: model.CriterionEmail, Value: "/.*/"},
		model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "contract", Value: "**"},
		model.IdentificationCriterion{Type: model.CriterionEmail, Value: `qa@example.com\`},
	)

	terms := make([]string, 0, len(crm.queries))
	for _, q := range crm.queries {
		terms = append(terms, q.Term)
		require.Equal(t, 1, q.DomainID)
		require.Equal(t, int32(crmPageSize), q.Size)
	}

	// The CRM splits terms on "*", and reads "%", a leading "=" and "/x/" as
	// operators: none may reach it. Terms left empty are not searched at all.
	// A phone is searched by its national tail (9 digits), so the number
	// stored without the country code is found too; rows are re-checked.
	// "/.*/" is not an email at all and never reaches the CRM.
	require.Equal(t, []string{"000000001", "qa@example.com", "qax@example.com", "contract=A-1", "qa@example.com"}, terms)
}

func TestIdentify_PagesThroughCRMResults(t *testing.T) {
	t.Parallel()

	rows := make([]*model.CRMContact, 0, 250)
	for i := range 250 {
		rows = append(rows, &model.CRMContact{ID: strconv.Itoa(1000 + i), Phones: []model.CRMPhone{{Number: "380000000500"}}})
	}

	rows[230].Phones = []model.CRMPhone{{Number: "380000000001"}}
	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: rows}}
	f := newIdentificationFixture(t, nil, crm)

	res := f.identify(t, phone("380000000001"))

	require.Equal(t, 1, res.FoundCount, "a match on the third page is found")
	require.Len(t, crm.queries, 3)
}

func TestIdentify_OperatorClearedBindingIsNotRebound(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
	f.threads.binding = model.CRMContactBinding{Source: model.CRMContactSourceSelected}

	res := f.identify(t)

	require.Empty(t, res.CRMContactID, "the candidate is not the thread's contact")
	require.Equal(t, 1, res.FoundCount)
	require.Empty(t, f.threads.setCRMContactCalls, "an operator's \"no contact\" stands")
}

func TestIdentify_AutoBindDoesNotTakeAnIMContactLinkedMeanwhile(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}}}
	crm.ownersLater = linkedTo(f.client.String())

	f.identify(t)

	require.Equal(t, []string{"20"}, f.threads.setCRMContactCalls)
	require.Empty(t, crm.links, "the link made meanwhile is not moved back")
}

func TestListCandidates_RunsChannelSearchOnce(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}

	view, err := f.svc.ListCandidates(context.Background(), f.threadID, 1)
	require.NoError(t, err)
	require.Len(t, view.Candidates, 1)
	require.Equal(t, model.CRMContactBinding{CRMContactID: "10", Source: model.CRMContactSourceAuto}, view.Binding)

	calls := len(crm.queries)
	_, err = f.svc.ListCandidates(context.Background(), f.threadID, 1)
	require.NoError(t, err)
	require.Equal(t, calls, len(crm.queries), "channel search runs once per thread")
}

func TestListCandidates_ChannelFailureStillLists(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{err: stderrors.New("crm down")}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)

	view, err := f.svc.ListCandidates(context.Background(), f.threadID, 1)
	require.NoError(t, err)
	require.Empty(t, view.Candidates)
	require.False(t, f.candidates.checked, "retried on the next read")
}

type fakeLinker struct {
	calls      []string
	newThreads []uuid.UUID
}

func (f *fakeLinker) IdentifyNewThread(_ context.Context, threadID uuid.UUID, _ int) {
	f.newThreads = append(f.newThreads, threadID)
}

func (f *fakeLinker) LinkChosenContact(_ context.Context, _ uuid.UUID, _ int, crmContactID string) {
	f.calls = append(f.calls, crmContactID)
}

func TestSetCRMContact_SourceAndEnrichment(t *testing.T) {
	t.Parallel()

	operator := uuid.New()
	members := []*model.ThreadDialogExtended{{ContactID: operator, ThreadRole: model.RoleMember}}

	tests := []struct {
		name       string
		crmID      string
		source     model.CRMContactSource
		noLink     bool
		wantCode   codes.Code
		wantSource []model.CRMContactSource
		wantLinks  []string
	}{
		{name: "unspecified means selected", crmID: "42", wantSource: []model.CRMContactSource{model.CRMContactSourceSelected}, wantLinks: []string{"42"}},
		{name: "created", crmID: "42", source: model.CRMContactSourceCreated, wantSource: []model.CRMContactSource{model.CRMContactSourceCreated}, wantLinks: []string{"42"}},
		{name: "caller without write access: bound, not linked", crmID: "42", noLink: true, wantSource: []model.CRMContactSource{model.CRMContactSourceSelected}},
		{name: "auto is reserved for identification", crmID: "42", source: model.CRMContactSourceAuto, wantCode: codes.InvalidArgument},
		{name: "unknown source", crmID: "42", source: 9, wantCode: codes.InvalidArgument},
		{name: "clearing links nothing", crmID: "", wantSource: []model.CRMContactSource{model.CRMContactSourceSelected}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			threads := &fakeThreadStore{}
			linker := &fakeLinker{}
			svc := NewThreadService(nil, fakeUnitOfWork{
				threadStore:       threads,
				threadDialogStore: &fakeThreadDialogStore{fullViewResult: members},
				outboxStore:       &fakeOutboxStore{},
			}, nil, nil, nil, &fakeTagHooks{}, nil, linker)

			err := svc.SetCRMContact(context.Background(), &dto.SetCRMContactRequest{
				ThreadID: uuid.New(), InitiatorContactID: operator, DomainID: 1, CRMContactID: tt.crmID, Source: tt.source,
				LinkIMContact: !tt.noLink,
			})

			if tt.wantCode != codes.OK {
				require.Equal(t, tt.wantCode, errors.Code(err))
				require.Empty(t, threads.setCRMContactCalls)
				require.Empty(t, linker.calls)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantSource, threads.setCRMContactSources)
			require.Equal(t, tt.wantLinks, linker.calls)
		})
	}
}

// The operator's choice gets the channel identifier only when no other CRM
// contact holds it; an existing link is never moved.
func TestLinkChosenContact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		owners    func(f *identificationFixture) []*model.CRMContact
		wantLinks int
	}{
		{name: "free identifier is added to the chosen contact", owners: func(*identificationFixture) []*model.CRMContact { return nil }, wantLinks: 1},
		{name: "identifier of another contact stays there", owners: func(f *identificationFixture) []*model.CRMContact {
			return []*model.CRMContact{{ID: "10", IMContactIDs: []string{f.client.String()}}}
		}},
		{name: "already linked to the chosen contact", owners: func(f *identificationFixture) []*model.CRMContact {
			return []*model.CRMContact{{ID: "77", IMContactIDs: []string{f.client.String()}}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{}
			f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "viber"}, crm)
			crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: tt.owners(f)}
			f.threads.binding = model.CRMContactBinding{CRMContactID: "77", Source: model.CRMContactSourceSelected}

			f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
			require.NoError(t, f.svc.Wait(context.Background()))
			require.Len(t, crm.links, tt.wantLinks)

			if tt.wantLinks > 0 {
				require.Equal(t, model.IMContactLink{DomainID: 1, CRMContactID: "77", IMContactID: f.client, Protocol: "viber", Via: f.via}, crm.links[0])
			}
		})
	}
}

func TestIdentifyNewThread_BindsInTheBackground(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}

	ctx, cancel := context.WithCancel(context.Background())
	f.svc.IdentifyNewThread(ctx, f.threadID, 1)
	cancel() // the request that created the thread is over; the search goes on
	require.NoError(t, f.svc.Wait(context.Background()))

	require.Equal(t, []string{"10"}, f.threads.setCRMContactCalls)
	require.True(t, f.candidates.checked)
}

func TestIdentifyNewThread_TooManyRunningIsDeferred(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)

	for range maxBackgroundRuns {
		f.svc.background <- struct{}{}
	}

	f.svc.IdentifyNewThread(context.Background(), f.threadID, 1)
	require.NoError(t, f.svc.Wait(context.Background()))

	require.Empty(t, crm.queries, "left to the first read")
}

func TestListCandidates_FailedChannelSearchBacksOff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity *model.ChannelIdentity
		crmErr   error
	}{
		{name: "crm down", identity: &model.ChannelIdentity{Type: "telegram"}, crmErr: stderrors.New("crm down")},
		{name: "no client on the thread", identity: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{err: tt.crmErr}
			f := newIdentificationFixture(t, tt.identity, crm)

			lookups := 0
			countingChannels := &countingChannelIdentities{inner: &fakeChannels{identity: tt.identity}, calls: &lookups}
			f.svc.channels = countingChannels

			now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
			f.svc.now = func() time.Time { return now }

			_, err := f.svc.ListCandidates(context.Background(), f.threadID, 1)
			require.NoError(t, err)

			first := lookups + len(crm.queries)
			require.Positive(t, first)

			now = now.Add(channelRetryAfter / 2)
			_, err = f.svc.ListCandidates(context.Background(), f.threadID, 1)
			require.NoError(t, err)
			require.Equal(t, first, lookups+len(crm.queries), "a read soon after does not search again")

			now = now.Add(channelRetryAfter)
			_, err = f.svc.ListCandidates(context.Background(), f.threadID, 1)
			require.NoError(t, err)
			require.Greater(t, lookups+len(crm.queries), first, "a later read retries")
			require.False(t, f.candidates.checked)
		})
	}
}

type countingChannelIdentities struct {
	inner ChannelIdentities
	calls *int
}

func (c *countingChannelIdentities) ChannelIdentity(ctx context.Context, id uuid.UUID, domainID int) (*model.ChannelIdentity, error) {
	*c.calls++

	return c.inner.ChannelIdentity(ctx, id, domainID)
}

// The IM contact's link moved to another CRM contact (an operator chose it):
// the old owner's messenger-id reason is stale, it stays a hint only.
func TestIdentify_MovedLinkDowngradesTheOldOwner(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
	f.identify(t)

	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: {{ID: "11", IMContactIDs: []string{f.client.String()}}}}
	f.threads.binding = model.CRMContactBinding{CRMContactID: "11", Source: model.CRMContactSourceSelected}
	res := f.identify(t)

	require.Equal(t, 2, res.FoundCount)
	require.Equal(t, model.TrustUnverified, f.candidates.rows["10"].Trust)
	require.Equal(t, model.TrustVerified, f.candidates.rows["11"].Trust)
	require.Empty(t, f.candidates.duplicates, "one verified contact is no duplicate")
}

// A binding an operator chose is never linked by identification: only
// SetCRMContact links it, and only for an operator who may change the contact.
// The chosen contact is the verified match of the channel's number and the IM
// contact is free, so only the binding's source keeps it from being linked.
func TestIdentify_DoesNotLinkAnOperatorsChoice(t *testing.T) {
	t.Parallel()

	for _, source := range []model.CRMContactSource{model.CRMContactSourceSelected, model.CRMContactSourceCreated, model.CRMContactSourceSearched, model.CRMContactSourceUnspecified} {
		crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{
			model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}},
		}}
		f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
		f.threads.binding = model.CRMContactBinding{CRMContactID: "20", Source: source}

		res := f.identify(t)

		require.Equal(t, "20", res.CRMContactID, "source %v: the verified bound contact is reported", source)
		require.Empty(t, crm.links, "source %v", source)
	}
}

// The automatic link is made once, to the bound contact; a second run finds
// the IM contact linked and does not link again.
func TestIdentify_AutoLinkOnce(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}}}

	f.identify(t)
	f.identify(t)

	require.Len(t, crm.links, 1)
	require.Equal(t, "20", crm.links[0].CRMContactID)
}

// During a rollout an older instance may change the contact without touching
// its source: a binding still marked auto but no longer a verified candidate
// is not linked (that would bypass the operator's write check).
func TestIdentify_StaleAutoSourceIsNotLinked(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}}}
	f.threads.binding = model.CRMContactBinding{CRMContactID: "77", Source: model.CRMContactSourceAuto}

	f.identify(t)

	require.Empty(t, crm.links, "77 is not a verified candidate of this thread")
}

// An operator's choice committed while identification searched (it held the
// row lock first) is what the run sees under the lock: nothing is bound over
// it and nothing is linked.
func TestIdentify_OperatorChoiceUnderTheLockWins(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}}}
	f.threads.onLock = func(ts *fakeThreadStore) {
		ts.binding = model.CRMContactBinding{CRMContactID: "42", Source: model.CRMContactSourceSelected}
	}

	res := f.identify(t)

	require.Empty(t, f.threads.setCRMContactCalls, "the operator's choice is not overwritten")
	require.Empty(t, crm.links, "nothing is linked for a binding the run did not make")
	require.Empty(t, res.CRMContactID, "the schema is not told about a contact that is not on the thread")
}

// A second choice that came first: the earlier choice is not linked.
func TestLinkChosenContact_SkipsAChoiceReplacedMeanwhile(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "viber"}, crm)
	f.threads.binding = model.CRMContactBinding{CRMContactID: "88", Source: model.CRMContactSourceSelected}

	f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
	require.NoError(t, f.svc.Wait(context.Background()))
	require.Empty(t, crm.links)
}

func TestIdentify_LastDigitsAreTheCRMTerm(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, nil, crm)

	_, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{
		ThreadID: f.threadID, DomainID: 1, Criteria: []model.IdentificationCriterion{phone("+380 00 000 12 34")}, PhoneLastDigits: 4,
	})
	require.NoError(t, err)
	require.Len(t, crm.queries, 1)
	require.Equal(t, model.CRMContactQuery{DomainID: 1, Field: model.CRMSearchPhones, Term: "1234", Size: crmPageSize, Page: 1}, crm.queries[0])
}

func TestIsChannelPhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want bool
	}{
		{"380000000001", true},
		{"+380 (00) 000-00-01", true},
		{"0000001", true},
		{"000001", false},
		{"1234567890123456", false},
		{"wa-1234567890", false},
		{"AbC1234567", false},
		{"", false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, isChannelPhone(tt.raw), tt.raw)
	}
}

// WhatsApp may hide the number and send an opaque id that holds digits: it is
// never searched as a phone, so it cannot become a verified match.
func TestIdentify_HiddenWhatsAppNumberIsNotSearched(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "1234567890"}}}},
	}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "wa-1234567890"}, crm)

	res := f.identify(t)

	require.Zero(t, res.FoundCount)
	require.Empty(t, f.threads.setCRMContactCalls)

	for _, q := range crm.queries {
		require.NotEqual(t, model.CRMSearchPhones, q.Field)
	}
}

// Criteria matches of a thread without a client are stored, but the thread is
// not marked searched: the client who joins later is searched on the next read.
func TestIdentify_NoClientDoesNotMarkTheChannelSearched(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchEmails: {{ID: "40", Emails: []string{"qa-ws-166@example.com"}}},
	}}
	f := newIdentificationFixture(t, nil, crm)

	res := f.identify(t, model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-ws-166@example.com"})

	require.Equal(t, 1, res.FoundCount)
	require.Contains(t, f.candidates.rows, "40")
	require.False(t, f.candidates.checked)
}

// The binding changed after the run committed (an operator chose another
// contact): the background link of the automatic binding is not written.
func TestIdentify_AutoLinkRechecksTheBindingAfterCommit(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchPhones: {{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}}},
	}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	// Reads: the domain check, the one under the lock, then the link's re-check.
	f.threads.onBindingRead = func(ts *fakeThreadStore, n int) {
		if n == 3 {
			ts.binding = model.CRMContactBinding{CRMContactID: "42", Source: model.CRMContactSourceSelected}
		}
	}

	f.identify(t)

	require.Equal(t, []string{"20"}, f.threads.setCRMContactCalls)
	require.Empty(t, crm.links)
}

// A run whose channel search started before the channel step another run
// already stored does not apply its older view of the CRM: the newer link
// state is neither downgraded nor overwritten, the criteria are still stored.
func TestIdentify_OutdatedChannelStepIsNotApplied(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchIMContact: linkedTo(f.client.String()),
		model.CRMSearchEmails:    {{ID: "40", Emails: []string{"qa-ws-166@example.com"}}},
	}

	messenger := model.MatchReason{Type: model.CriterionMessengerID, Trust: model.TrustVerified, Origin: model.OriginChannel}
	f.candidates.rows = map[string]*model.ContactCandidate{
		"11": {ThreadID: f.threadID, DomainID: 1, CRMContactID: "11", Trust: model.TrustVerified, Reasons: []model.MatchReason{messenger}},
	}
	f.threads.binding = model.CRMContactBinding{CRMContactID: "11", Source: model.CRMContactSourceAuto}
	newer := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f.candidates.checked, f.candidates.checkedAt = true, newer

	res := f.identify(t, model.IdentificationCriterion{Type: model.CriterionEmail, Value: "qa-ws-166@example.com"})

	require.Equal(t, 2, res.FoundCount, "the stored candidate and the criterion match")
	require.Equal(t, model.TrustVerified, f.candidates.rows["11"].Trust, "the newer link state stands")
	require.NotContains(t, f.candidates.rows, "10", "the older owners are not applied")
	require.Contains(t, f.candidates.rows, "40")
	require.Equal(t, newer, f.candidates.checkedAt)
	require.Empty(t, f.threads.setCRMContactCalls)
	require.Empty(t, crm.links)
}

func TestLinkChosenContact_AlreadyLinkedMeanwhileIsNotAFailure(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{}, linkErr: fmt.Errorf("crm: %w", model.ErrIMContactLinked)}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "viber"}, crm)
	f.threads.binding = model.CRMContactBinding{CRMContactID: "77", Source: model.CRMContactSourceSelected}

	f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
	require.NoError(t, f.svc.Wait(context.Background()))

	require.Len(t, crm.links, 1)
	require.Contains(t, f.logs.String(), "linked to a crm contact meanwhile")
	require.NotContains(t, f.logs.String(), "level=WARN")
}

func TestLinkChosenContact_BoundedInFlight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		running func(f *identificationFixture) []string
		wantLog string
	}{
		{
			name:    "the same link already running",
			running: func(f *identificationFixture) []string { return []string{f.threadID.String() + "/77"} },
			wantLog: "im contact link already running",
		},
		{
			name: "too many links running",
			running: func(*identificationFixture) []string {
				keys := make([]string, 0, maxBackgroundLinks)
				for range maxBackgroundLinks {
					keys = append(keys, uuid.NewString()+"/1")
				}

				return keys
			},
			wantLog: "im contact link skipped: too many running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{}}
			f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "viber"}, crm)
			f.threads.binding = model.CRMContactBinding{CRMContactID: "77", Source: model.CRMContactSourceSelected}

			for _, key := range tt.running(f) {
				f.svc.links[key] = struct{}{}
			}

			f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
			require.NoError(t, f.svc.Wait(context.Background()))

			require.Empty(t, crm.links)
			require.Contains(t, f.logs.String(), tt.wantLog)
		})
	}
}

func TestLinkChosenContact_ReleasesItsSlot(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{}, linkErr: stderrors.New("crm down")}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "viber"}, crm)
	f.threads.binding = model.CRMContactBinding{CRMContactID: "77", Source: model.CRMContactSourceSelected}

	f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
	require.NoError(t, f.svc.Wait(context.Background()))
	require.Empty(t, f.svc.links, "a failed link frees its slot")
	require.Contains(t, f.logs.String(), "linking im contact to the crm contact failed")

	crm.linkErr = nil

	f.svc.LinkChosenContact(context.Background(), f.threadID, 1, "77")
	require.NoError(t, f.svc.Wait(context.Background()))
	require.Len(t, crm.links, 2, "the next choice links again")
}

func TestListCandidates_ConcurrentReadDoesNotSearchAgain(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}
	f.svc.reading[f.threadID] = struct{}{} // another read runs the search right now

	view, err := f.svc.ListCandidates(context.Background(), f.threadID, 1)
	require.NoError(t, err)
	require.Empty(t, view.Candidates, "the stored list, as it is")
	require.Empty(t, crm.queries)
}

func TestEndChannelRead_FullBackoffForgetsTheOldestOnly(t *testing.T) {
	t.Parallel()

	f := newIdentificationFixture(t, nil, &fakeCRM{})
	now := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	f.svc.now = func() time.Time { return now }

	oldest := uuid.New()
	f.svc.backoff[oldest] = now.Add(-channelRetryAfter / 2)

	for i := 1; i < maxChannelBackoff; i++ {
		f.svc.backoff[uuid.New()] = now.Add(-channelRetryAfter/2 + time.Duration(i)*time.Microsecond)
	}

	failed := uuid.New()
	f.svc.reading[failed] = struct{}{}
	f.svc.endChannelRead(failed, false)

	require.Len(t, f.svc.backoff, maxChannelBackoff)
	require.NotContains(t, f.svc.backoff, oldest)
	require.Contains(t, f.svc.backoff, failed)
	require.NotContains(t, f.svc.reading, failed)
}

func TestShutdown_CancelsBackgroundWork(t *testing.T) {
	t.Parallel()

	f := newIdentificationFixture(t, nil, &fakeCRM{})

	started := make(chan struct{})

	var cause error

	f.svc.inBackground(context.Background(), time.Hour, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		cause = ctx.Err()
	})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, f.svc.Shutdown(ctx))
	require.ErrorIs(t, cause, context.Canceled)
}

// The CRM fails every criterion but answered the channel search: the verified
// channel match is still stored and bound, and the schema is told the run failed.
func TestIdentify_EveryCriterionFailingStillStoresTheChannelStep(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{errByField: map[model.CRMSearchField]error{model.CRMSearchEmails: stderrors.New("crm down")}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}

	_, err := f.svc.Identify(context.Background(), &model.IdentifyContactCommand{
		ThreadID: f.threadID, DomainID: 1, Criteria: []model.IdentificationCriterion{{Type: model.CriterionEmail, Value: "qa-ws-166@example.com"}},
	})
	require.NoError(t, f.svc.Wait(context.Background()))

	require.Equal(t, codes.Internal, errors.Code(err))
	require.Contains(t, f.candidates.rows, "10")
	require.Equal(t, []string{"10"}, f.threads.setCRMContactCalls)
	require.True(t, f.candidates.checked)
}

// Values are bounded in characters, like the API: a long value in Cyrillic is
// searched as long as it has at most maxCriterionValueLen letters.
func TestUnusable_ValueLengthInCharacters(t *testing.T) {
	t.Parallel()

	fits := strings.Repeat("я", maxCriterionValueLen)
	tooLong := fits + "я"

	require.Empty(t, unusable(model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "note", Value: fits}))
	require.NotEmpty(t, unusable(model.IdentificationCriterion{Type: model.CriterionAttribute, Key: "note", Value: tooLong}))
}

func TestListCandidates_ReadSearchesAreBounded(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "telegram"}, crm)
	crm.byField = map[model.CRMSearchField][]*model.CRMContact{model.CRMSearchIMContact: linkedTo(f.client.String())}

	for range maxChannelReads {
		f.svc.reading[uuid.New()] = struct{}{}
	}

	_, err := f.svc.ListCandidates(context.Background(), f.threadID, 1)
	require.NoError(t, err)
	require.Empty(t, crm.queries, "the stored list, no search")
}

// An automatic binding while two verified candidates exist is not one this
// service made (an older instance changed the contact): it is not linked.
func TestIdentify_AutoSourceWithTwoVerifiedIsNotLinked(t *testing.T) {
	t.Parallel()

	crm := &fakeCRM{byField: map[model.CRMSearchField][]*model.CRMContact{
		model.CRMSearchPhones: {
			{ID: "20", Phones: []model.CRMPhone{{Number: "380000000001"}}},
			{ID: "21", Phones: []model.CRMPhone{{Number: "+380 00 000 00 01"}}},
		},
	}}
	f := newIdentificationFixture(t, &model.ChannelIdentity{Type: "whatsapp", Phone: "380000000001"}, crm)
	f.threads.binding = model.CRMContactBinding{CRMContactID: "20", Source: model.CRMContactSourceAuto}

	f.identify(t)

	require.Empty(t, crm.links)
	require.Len(t, f.candidates.duplicates, 1)
}
