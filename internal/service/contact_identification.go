package service

import (
	"cmp"
	"context"
	stderrors "errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

const (
	// crmPageSize and crmMaxPages bound one CRM search. The CRM matches
	// substrings, so every row is re-checked in Go: a broader CRM match costs
	// rows, never a wrong candidate.
	crmPageSize = 100
	crmMaxPages = 5
	// crmSearchTimeout bounds the CRM calls of one Identify run.
	crmSearchTimeout = 10 * time.Second
	// channelReadTimeout bounds the channel search a first ListCandidates runs,
	// so a slow CRM delays the operator's read only briefly.
	channelReadTimeout = 3 * time.Second
	// linkTimeout bounds the CRM link written after a binding is committed. It
	// runs detached from the request: the binding is already stored. The
	// transport retries an unavailable CRM within it.
	linkTimeout = 5 * time.Second
	// maxBackgroundLinks bounds the links written at once; past it a link is
	// skipped (and logged) until the binding is made again.
	maxBackgroundLinks = 64
	// maxCriteria bounds the criteria of one run, like the API does.
	maxCriteria = 16
	// maxCriterionValueLen bounds one value, like the API does.
	maxCriterionValueLen = 256
	// minPhoneDigits keeps a phone search from matching most of the CRM.
	minPhoneDigits = 4
	// minEmailLen keeps a one-letter "email" from scanning the CRM.
	minEmailLen = 5
	// phoneSearchTail is how many trailing digits are sent to the CRM when no
	// last-N is asked: the CRM finds a number only if the term is inside it, so
	// the whole international number would miss the same number stored in its
	// local form. The whole number is still what rows are compared with.
	phoneSearchTail = 9
	// A channel phone outside these bounds is not a phone number (WhatsApp may
	// hide the number and send an opaque id instead).
	minChannelPhoneDigits = 7
	maxChannelPhoneDigits = 15
	// maxMemberLookups bounds the IM contact lookups made to find the client.
	maxMemberLookups = 5
	// maxChannelReads bounds the channel searches reads run at once; past it a
	// read lists what is stored (after a restart many threads are unsearched).
	maxChannelReads = 32
	// maxBackgroundRuns bounds the channel searches started for new threads
	// at once; past it a thread is left to the search its first read runs.
	maxBackgroundRuns = 16
	// channelRetryAfter is how long a read does not repeat a channel search
	// that failed or found no client, so a CRM outage or a thread without a
	// client does not slow every read of the list.
	channelRetryAfter = 30 * time.Second
	// maxChannelBackoff bounds the remembered failed searches.
	maxChannelBackoff = 10_000
	// backgroundRunTimeout bounds one background channel search: the CRM
	// search and the transaction storing it.
	backgroundRunTimeout = crmSearchTimeout + 5*time.Second
	// whatsappChannel is the IM contact type whose phone is the WhatsApp number.
	whatsappChannel = "whatsapp"
	// internalIssuer issues the IM contacts of Webitel users (operators).
	internalIssuer = "webitel"
)

// CRMContacts is the CRM side of identification.
type CRMContacts interface {
	// SearchContacts returns one page and whether more pages follow.
	SearchContacts(ctx context.Context, q model.CRMContactQuery) (contacts []*model.CRMContact, next bool, err error)
	LinkIMContact(ctx context.Context, link model.IMContactLink) error
}

// ChannelIdentities resolves the IM contact of the thread's external member.
type ChannelIdentities interface {
	ChannelIdentity(ctx context.Context, contactID uuid.UUID, domainID int) (*model.ChannelIdentity, error)
}

// ContactCandidatesView is the thread's candidate list with its binding.
type ContactCandidatesView struct {
	Candidates model.ContactCandidates
	Binding    model.CRMContactBinding
}

// ContactIdentificationService matches threads to CRM contacts.
type ContactIdentificationService struct {
	uow      store.UnitOfWork
	crm      CRMContacts
	channels ChannelIdentities
	logger   *slog.Logger
	now      func() time.Time

	// background holds a slot per channel search running for a new thread.
	background chan struct{}
	// wg tracks the background work (searches and links), so tests and
	// shutdown can wait for it.
	wg sync.WaitGroup
	// stopped ends the background work on shutdown.
	stopped context.Context //nolint:containedctx // the service's lifecycle, not a request's context
	stop    context.CancelFunc

	linksMu sync.Mutex
	// links holds the links being written, by thread and CRM contact.
	links map[string]struct{}

	backoffMu sync.Mutex
	// backoff is when a read last ran a channel search that failed or found
	// no client, per thread.
	backoff map[uuid.UUID]time.Time
	// reading holds the threads whose channel search a read runs right now.
	reading map[uuid.UUID]struct{}
}

func NewContactIdentificationService(uow store.UnitOfWork, crm CRMContacts, channels ChannelIdentities, logger *slog.Logger) *ContactIdentificationService {
	if logger == nil {
		logger = slog.Default()
	}

	stopped, stop := context.WithCancel(context.Background())

	return &ContactIdentificationService{
		uow:      uow,
		crm:      crm,
		channels: channels,
		logger:   logger.With(slog.String("component", "contact_identification")),
		now:      func() time.Time { return time.Now().UTC() },

		background: make(chan struct{}, maxBackgroundRuns),
		stopped:    stopped,
		stop:       stop,
		links:      make(map[string]struct{}),
		backoff:    make(map[uuid.UUID]time.Time),
		reading:    make(map[uuid.UUID]struct{}),
	}
}

// errNoExternalMember means the thread has no client to take channel identifiers from.
var errNoExternalMember = stderrors.New("thread has no external member")

// externalMember is the thread's client side: the IM contact and the gate it writes through.
type externalMember struct {
	identity *model.ChannelIdentity
	via      string
}

// Identify searches the channel identifiers and the command's criteria,
// appends the matches to the thread's candidates and applies the binding rules.
// A criterion that cannot be searched (what a client typed instead of a phone,
// say) is skipped, so the others and the channel search still run.
func (s *ContactIdentificationService) Identify(ctx context.Context, cmd *model.IdentifyContactCommand) (*model.IdentifyContactResult, error) {
	const errID = "service.contact_identification.identify"

	if err := validateIdentify(cmd); err != nil {
		return nil, err
	}

	// Domain check before any CRM call: NotFound for another domain's thread.
	if _, err := s.uow.ThreadStore().GetCRMContactBinding(ctx, cmd.ThreadID, cmd.DomainID); err != nil {
		return nil, err
	}

	run := *cmd
	run.Criteria = s.usableCriteria(ctx, cmd)

	all, binding, _, err := s.run(ctx, &run, crmSearchTimeout)
	if err != nil {
		return nil, errors.Internal("identifying contact", errors.WithCause(err), errors.WithID(errID))
	}

	result := &model.IdentifyContactResult{FoundCount: len(all), Trust: all.MaxTrust()}
	if len(all) == 1 && all[0].Trust == model.TrustVerified && all[0].CRMContactID == binding.CRMContactID {
		result.CRMContactID = all[0].CRMContactID
	}

	return result, nil
}

// IdentifyNewThread starts the channel search of a thread just created
// (always, without a schema), so a client known to the CRM is bound before any
// operator or schema asks, and returns at once: thread creation never waits
// for the CRM. When too many searches already run, the thread is left to the
// search its first ListCandidates runs.
func (s *ContactIdentificationService) IdentifyNewThread(ctx context.Context, threadID uuid.UUID, domainID int) {
	select {
	case s.background <- struct{}{}:
	default:
		s.logger.DebugContext(ctx, "channel identification deferred to the first read: too many running",
			slog.String("thread_id", threadID.String()))

		return
	}

	s.inBackground(ctx, backgroundRunTimeout, func(ctx context.Context) {
		defer func() { <-s.background }()

		cmd := &model.IdentifyContactCommand{ThreadID: threadID, DomainID: domainID}
		if _, _, _, err := s.run(ctx, cmd, crmSearchTimeout); err != nil {
			s.logger.WarnContext(ctx, "channel identification of a new thread failed; the first read retries it",
				slog.String("thread_id", threadID.String()), slog.Any("err", err))
		}
	})
}

// Shutdown cancels the background work and waits for it to end, or for ctx;
// the service calls it on stop, before the CRM client and the database pool
// close.
func (s *ContactIdentificationService) Shutdown(ctx context.Context) error {
	s.stop()

	return s.Wait(ctx)
}

// Wait blocks until the background work finishes or ctx ends.
func (s *ContactIdentificationService) Wait(ctx context.Context) error {
	done := make(chan struct{})

	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ListCandidates returns the thread's candidates. If the channel identifiers
// were not searched yet (the search at thread creation failed, or the thread
// is older than this feature), the read runs that search first.
func (s *ContactIdentificationService) ListCandidates(ctx context.Context, threadID uuid.UUID, domainID int) (*ContactCandidatesView, error) {
	const errID = "service.contact_identification.list_candidates"

	if threadID == uuid.Nil || domainID <= 0 {
		return nil, errors.InvalidArgument("thread_id and domain_id are required", errors.WithID(errID))
	}

	binding, err := s.uow.ThreadStore().GetCRMContactBinding(ctx, threadID, domainID)
	if err != nil {
		return nil, err
	}

	_, checked, err := s.uow.ContactCandidates().ChannelCheckedAt(ctx, threadID)
	if err != nil {
		return nil, err
	}

	if !checked && s.startChannelRead(threadID) {
		all, bound, searched, err := s.run(ctx, &model.IdentifyContactCommand{ThreadID: threadID, DomainID: domainID}, channelReadTimeout)
		s.endChannelRead(threadID, err == nil && searched)

		if err == nil {
			return &ContactCandidatesView{Candidates: all, Binding: bound}, nil
		}

		// The list is still useful without the channel step; a later read retries it.
		s.logger.WarnContext(ctx, "channel identification failed", slog.String("thread_id", threadID.String()), slog.Any("err", err))
	}

	candidates, err := s.uow.ContactCandidates().List(ctx, threadID, domainID)
	if err != nil {
		return nil, err
	}

	return &ContactCandidatesView{Candidates: candidates, Binding: binding}, nil
}

// startChannelRead reports whether a read may run the thread's channel search
// now: no other read runs it, fewer than maxChannelReads run at all, and none
// failed or found no client within channelRetryAfter. A read that may not just
// lists what is stored.
func (s *ContactIdentificationService) startChannelRead(threadID uuid.UUID) bool {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()

	if _, running := s.reading[threadID]; running || len(s.reading) >= maxChannelReads {
		return false
	}

	if at, ok := s.backoff[threadID]; ok && s.now().Sub(at) < channelRetryAfter {
		return false
	}

	s.reading[threadID] = struct{}{}

	return true
}

// endChannelRead ends a read's channel search; one that failed or found no
// client is not repeated by reads within channelRetryAfter.
func (s *ContactIdentificationService) endChannelRead(threadID uuid.UUID, done bool) {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()

	delete(s.reading, threadID)

	if done {
		delete(s.backoff, threadID)

		return
	}

	now := s.now()

	if len(s.backoff) >= maxChannelBackoff {
		maps.DeleteFunc(s.backoff, func(_ uuid.UUID, at time.Time) bool { return now.Sub(at) >= channelRetryAfter })
	}

	// Still full (a long CRM outage): forget the oldest one, not all of them,
	// so the others keep sparing the CRM.
	if len(s.backoff) >= maxChannelBackoff {
		oldest, oldestAt := uuid.Nil, now

		for id, at := range s.backoff {
			if !at.After(oldestAt) {
				oldest, oldestAt = id, at
			}
		}

		delete(s.backoff, oldest)
	}

	s.backoff[threadID] = now
}

// LinkChosenContact links the thread's IM contact to the CRM contact an
// operator chose: only when no other CRM contact holds it, never moving a
// link. The binding itself is already stored by the caller; the link is made
// in the background, so the operator's request does not wait for the CRM.
func (s *ContactIdentificationService) LinkChosenContact(ctx context.Context, threadID uuid.UUID, domainID int, crmContactID string) {
	if crmContactID == "" {
		return
	}

	s.linkInBackground(ctx, threadID, crmContactID, func(ctx context.Context) error {
		return s.linkChosen(ctx, threadID, domainID, crmContactID)
	})
}

// linkChosen links unless the thread is bound to another contact by now (a
// second choice that came first) or another CRM contact holds the IM contact.
func (s *ContactIdentificationService) linkChosen(ctx context.Context, threadID uuid.UUID, domainID int, crmContactID string) error {
	binding, err := s.uow.ThreadStore().GetCRMContactBinding(ctx, threadID, domainID)
	if err != nil {
		return err
	}

	if binding.CRMContactID != crmContactID {
		return nil
	}

	member, err := s.externalMember(ctx, threadID, domainID)
	if stderrors.Is(err, errNoExternalMember) {
		return nil
	}

	if err != nil {
		return err
	}

	owners, err := s.imContactOwners(ctx, domainID, member.identity.IMContactID.String())
	if err != nil {
		return err
	}

	if len(owners) > 0 {
		if !slices.Contains(owners, crmContactID) {
			s.logger.InfoContext(ctx, "im contact belongs to another crm contact; not linked to the chosen one",
				slog.String("thread_id", threadID.String()), slog.String("crm_contact_id", crmContactID))
		}

		return nil
	}

	return s.link(ctx, domainID, crmContactID, member)
}

// run performs one identification and returns the thread's full candidate
// list and the binding after it. searched reports whether the thread's client
// was found and its channel identifiers searched. When every criterion failed
// the channel step is still stored and bound, and that failure is returned.
//
// The channel step is an observation of the CRM at the time the run started.
// Runs on one thread are serialized only by the row lock taken after their
// searches, so a run that started before the channel step stored by another
// one does not apply its own (older) view: it would undo a fresher link state.
func (s *ContactIdentificationService) run(ctx context.Context, cmd *model.IdentifyContactCommand, timeout time.Duration) (all model.ContactCandidates, binding model.CRMContactBinding, searched bool, err error) {
	var none model.CRMContactBinding

	observedAt := s.now()

	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	member, err := s.externalMember(searchCtx, cmd.ThreadID, cmd.DomainID)
	if stderrors.Is(err, errNoExternalMember) {
		member, err = nil, nil
	}

	if err != nil {
		return nil, none, false, err
	}

	channelMatches, owners, err := s.channelMatches(searchCtx, cmd.DomainID, member)
	if err != nil {
		return nil, none, false, err
	}

	criteriaMatches, criteriaErr := s.criteriaMatches(searchCtx, cmd)

	// Nothing to store for a thread without a client and without matches: do
	// not take the thread lock that message sending also needs.
	if member == nil && len(criteriaMatches) == 0 {
		all, binding, err = s.current(ctx, cmd.ThreadID, cmd.DomainID)

		return all, binding, false, cmp.Or(err, criteriaErr)
	}

	var (
		threadID = cmd.ThreadID
		// outdated: another run stored a channel step newer than this one's.
		outdated bool
	)

	err = s.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
		if err := uow.ThreadStore().LockForUpdate(ctx, threadID); err != nil {
			return err
		}

		var err error

		if binding, err = uow.ThreadStore().GetCRMContactBinding(ctx, threadID, cmd.DomainID); err != nil {
			return err
		}

		checkedAt, checked, err := uow.ContactCandidates().ChannelCheckedAt(ctx, threadID)
		if err != nil {
			return err
		}

		outdated = member != nil && checked && checkedAt.After(observedAt)

		matches := criteriaMatches
		if member != nil && !outdated {
			matches = slices.Concat(channelMatches, criteriaMatches)
		}

		existing, err := uow.ContactCandidates().List(ctx, threadID, cmd.DomainID)
		if err != nil {
			return err
		}

		var changed model.ContactCandidates

		all, changed = model.MergeCandidates(existing, matches, owners, member != nil && !outdated, threadID, cmd.DomainID, s.now())
		if err := uow.ContactCandidates().Upsert(ctx, changed); err != nil {
			return err
		}

		// Marked only once a client was searched: a thread whose client joins
		// later is searched again on the next read.
		if member != nil && !outdated {
			if err := uow.ContactCandidates().MarkChannelChecked(ctx, threadID, cmd.DomainID, observedAt); err != nil {
				return err
			}
		}

		verified := all.Verified()
		if err := uow.ContactCandidates().SaveDuplicate(ctx, model.NewContactDuplicate(threadID, cmd.DomainID, verified)); err != nil {
			return err
		}

		bound := shouldAutoBind(binding, verified)
		if bound {
			binding = model.CRMContactBinding{CRMContactID: verified[0].CRMContactID, Source: model.CRMContactSourceAuto}

			if err := uow.ThreadStore().SetCRMContact(ctx, threadID, binding.CRMContactID, binding.Source); err != nil {
				return err
			}
		}

		if len(changed) == 0 && !bound {
			return nil
		}

		return s.publishUpdated(ctx, uow, threadID, cmd.DomainID, binding.CRMContactID)
	})
	if err != nil {
		return nil, none, false, err
	}

	if !outdated {
		s.ensureLink(ctx, threadID, cmd.DomainID, binding, member, owners, all)
	}

	s.logger.DebugContext(ctx, "contact identification done",
		slog.String("thread_id", threadID.String()),
		slog.Int("criteria", len(cmd.Criteria)),
		slog.Int("channel_matches", len(channelMatches)),
		slog.Int("criteria_matches", len(criteriaMatches)),
		slog.Bool("outdated_channel_step", outdated),
		slog.Int("candidates", len(all)),
		slog.String("bound_to", binding.CRMContactID))

	return all, binding, member != nil, criteriaErr
}

// current reads the stored candidates and binding without changing anything.
func (s *ContactIdentificationService) current(ctx context.Context, threadID uuid.UUID, domainID int) (model.ContactCandidates, model.CRMContactBinding, error) {
	binding, err := s.uow.ThreadStore().GetCRMContactBinding(ctx, threadID, domainID)
	if err != nil {
		return nil, binding, err
	}

	all, err := s.uow.ContactCandidates().List(ctx, threadID, domainID)
	if err != nil {
		return nil, binding, err
	}

	return all, binding, nil
}

// ensureLink writes the enrichment link of an automatic binding whose IM
// contact no CRM contact holds: right after the auto-bind, and again on a
// later run of the thread (the schema's node) when an earlier attempt failed.
// An operator's choice is linked only by SetCRMContact, and only when the
// operator may change the contact (LinkChosenContact); identification never
// links it. A failure is logged and the binding stands.
func (s *ContactIdentificationService) ensureLink(ctx context.Context, threadID uuid.UUID, domainID int, binding model.CRMContactBinding, member *externalMember, owners []string, all model.ContactCandidates) {
	if member == nil || binding.CRMContactID == "" || len(owners) > 0 || binding.Source != model.CRMContactSourceAuto {
		return
	}

	// An automatic binding is made only for the single verified candidate. Any
	// other one was changed by something unaware of the source (an older
	// instance during a rollout), or another verified candidate turned up since:
	// it is not linked without the operator.
	if verified := all.Verified(); len(verified) != 1 || verified[0].CRMContactID != binding.CRMContactID {
		return
	}

	// In the background: the schema or operator waiting on this run must not
	// also wait for the CRM write; the binding is already committed.
	s.linkInBackground(ctx, threadID, binding.CRMContactID, func(ctx context.Context) error {
		return s.linkUnowned(ctx, threadID, domainID, binding, member)
	})
}

// linkInBackground writes a link detached from the request. The same link of
// a thread is written once at a time, and at most maxBackgroundLinks at once.
// A skipped or failed link is not retried here (the transport retries an
// unavailable CRM): it is made when the binding is made again, by the
// schema's node, the client's next thread or the operator's next choice.
func (s *ContactIdentificationService) linkInBackground(ctx context.Context, threadID uuid.UUID, crmContactID string, link func(ctx context.Context) error) {
	key := threadID.String() + "/" + crmContactID
	attrs := []any{slog.String("thread_id", threadID.String()), slog.String("crm_contact_id", crmContactID)}

	s.linksMu.Lock()
	_, running := s.links[key]
	full := len(s.links) >= maxBackgroundLinks

	if !running && !full {
		s.links[key] = struct{}{}
	}
	s.linksMu.Unlock()

	switch {
	case running:
		s.logger.DebugContext(ctx, "im contact link already running", attrs...)

		return
	case full:
		s.logger.WarnContext(ctx, "im contact link skipped: too many running", attrs...)

		return
	}

	s.inBackground(ctx, linkTimeout, func(ctx context.Context) {
		defer func() {
			s.linksMu.Lock()
			delete(s.links, key)
			s.linksMu.Unlock()
		}()

		err := link(ctx)

		switch {
		case err == nil:
		case stderrors.Is(err, model.ErrIMContactLinked):
			// Linked by someone else since the check: never moved.
			s.logger.InfoContext(ctx, "im contact was linked to a crm contact meanwhile; not linked", attrs...)
		default:
			s.logger.WarnContext(ctx, "linking im contact to the crm contact failed", append(attrs, slog.Any("err", err))...)
		}
	})
}

// inBackground runs fn detached from the request, bounded by timeout, ended
// by Shutdown and tracked by Wait.
func (s *ContactIdentificationService) inBackground(ctx context.Context, timeout time.Duration, fn func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	unhook := context.AfterFunc(s.stopped, cancel)

	s.wg.Go(func() {
		defer cancel()
		defer unhook()

		fn(ctx)
	})
}

// linkUnowned links the IM contact to the bound CRM contact unless, since the
// search, the binding changed or someone linked the IM contact elsewhere. The
// link is only inserted, never moved: one made after this check makes the CRM
// reject the insert instead of being taken over.
func (s *ContactIdentificationService) linkUnowned(ctx context.Context, threadID uuid.UUID, domainID int, want model.CRMContactBinding, member *externalMember) error {
	binding, err := s.uow.ThreadStore().GetCRMContactBinding(ctx, threadID, domainID)
	if err != nil {
		return err
	}

	if binding != want {
		return nil
	}

	owners, err := s.imContactOwners(ctx, domainID, member.identity.IMContactID.String())
	if err != nil || len(owners) > 0 {
		return err
	}

	return s.link(ctx, domainID, want.CRMContactID, member)
}

// shouldAutoBind binds a single verified candidate when no contact is bound
// yet. A manual binding is never replaced and an unverified match is never
// bound without an operator.
func shouldAutoBind(binding model.CRMContactBinding, verified model.ContactCandidates) bool {
	return len(verified) == 1 && binding.CRMContactID == "" && !binding.IsManual()
}

// externalMember finds the thread's client. Active members are preferred over
// those who left. The gate id may sit on the client's row, the bot's row or
// both (see ThreadDialogs.ExtractExternalPeers), and a direct thread can carry
// it on an operator's row too, so the client is the first non-bot member whose
// IM contact was not issued by Webitel itself.
func (s *ContactIdentificationService) externalMember(ctx context.Context, threadID uuid.UUID, domainID int) (*externalMember, error) {
	for _, includeLeft := range []bool{false, true} {
		dialogs, err := s.uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{
			ThreadIDs:      []uuid.UUID{threadID},
			DomainID:       domainID,
			IncludeDeleted: includeLeft,
		})
		if err != nil {
			return nil, err
		}

		member, err := s.pickExternal(ctx, dialogs, domainID)
		if err != nil || member != nil {
			return member, err
		}
	}

	return nil, errNoExternalMember
}

func (s *ContactIdentificationService) pickExternal(ctx context.Context, dialogs []*model.ThreadDialog, domainID int) (*externalMember, error) {
	var (
		gateVia string
		humans  []*model.ThreadDialog
	)

	for _, d := range dialogs {
		if d == nil {
			continue
		}

		if gateVia == "" && d.Via != nil && *d.Via != "" {
			gateVia = *d.Via
		}

		if !d.IsBot {
			humans = append(humans, d)
		}
	}

	if gateVia == "" {
		// No gate on the thread: an internal conversation, nobody to identify.
		return nil, nil //nolint:nilnil // "no external member" is not an error here
	}

	// Rows carrying the gate first (normally the client), then the oldest.
	slices.SortStableFunc(humans, func(a, b *model.ThreadDialog) int {
		return cmp.Or(
			cmp.Compare(viaRank(a), viaRank(b)),
			a.CreatedAt.Compare(b.CreatedAt),
		)
	})

	for _, d := range humans[:min(len(humans), maxMemberLookups)] {
		identity, err := s.channels.ChannelIdentity(ctx, d.ContactID, domainID)
		if stderrors.Is(err, model.ErrChannelIdentityNotFound) {
			continue
		}

		if err != nil {
			return nil, err
		}

		if identity.Issuer == internalIssuer {
			continue
		}

		via := gateVia
		if d.Via != nil && *d.Via != "" {
			via = *d.Via
		}

		return &externalMember{identity: identity, via: via}, nil
	}

	return nil, nil //nolint:nilnil // "no external member" is not an error here
}

func viaRank(d *model.ThreadDialog) int {
	if d.Via != nil && *d.Via != "" {
		return 0
	}

	return 1
}

// channelMatches searches the channel identifiers (verified). owners are
// the CRM contacts the IM contact is already linked to.
func (s *ContactIdentificationService) channelMatches(ctx context.Context, domainID int, member *externalMember) (matches []model.ContactMatch, owners []string, err error) {
	if member == nil {
		return nil, nil, nil
	}

	owners, err = s.imContactOwners(ctx, domainID, member.identity.IMContactID.String())
	if err != nil {
		return nil, nil, err
	}

	for _, id := range owners {
		matches = append(matches, model.ContactMatch{CRMContactID: id, Reason: model.MatchReason{
			Type: model.CriterionMessengerID, Trust: model.TrustVerified, Origin: model.OriginChannel,
		}})
	}

	if member.identity.Type == whatsappChannel && isChannelPhone(member.identity.Phone) {
		exact, tail, err := s.searchChannelPhone(ctx, domainID, member.identity.Phone)
		if err != nil {
			return nil, nil, err
		}

		// Only the very same number is verified and may be bound. A stored
		// number that is just its tail (no country code) could belong to
		// another country, so it stays a possible match.
		for _, id := range exact {
			matches = append(matches, model.ContactMatch{CRMContactID: id, Reason: model.MatchReason{
				Type: model.CriterionPhone, Trust: model.TrustVerified, Origin: model.OriginChannel,
			}})
		}

		for _, id := range tail {
			matches = append(matches, model.ContactMatch{CRMContactID: id, Reason: model.MatchReason{
				Type: model.CriterionPhone, Trust: model.TrustUnverified, Origin: model.OriginChannel,
			}})
		}
	}

	return matches, owners, nil
}

// isChannelPhone reports whether the channel's "phone" is a phone number: only
// digits and separators, 7 to 15 digits. A hidden number arrives as an opaque id.
func isChannelPhone(raw string) bool {
	if strings.ContainsFunc(raw, func(r rune) bool {
		return (r < '0' || r > '9') && !strings.ContainsRune("+ -()", r)
	}) {
		return false
	}

	n := len(model.PhoneDigits(raw))

	return n >= minChannelPhoneDigits && n <= maxChannelPhoneDigits
}

// usableCriteria drops the criteria that cannot be searched and caps their
// number. Values are logged masked.
func (s *ContactIdentificationService) usableCriteria(ctx context.Context, cmd *model.IdentifyContactCommand) []model.IdentificationCriterion {
	out := make([]model.IdentificationCriterion, 0, min(len(cmd.Criteria), maxCriteria))

	for _, c := range cmd.Criteria {
		if len(out) == maxCriteria {
			s.logger.WarnContext(ctx, "contact identification: criteria over the limit are ignored",
				slog.String("thread_id", cmd.ThreadID.String()), slog.Int("criteria", len(cmd.Criteria)), slog.Int("limit", maxCriteria))

			break
		}

		c.Value = strings.TrimSpace(c.Value)

		if reason := unusable(c); reason != "" {
			s.logger.DebugContext(ctx, "contact identification: criterion skipped",
				slog.String("thread_id", cmd.ThreadID.String()),
				slog.Int("criterion_type", int(c.Type)),
				slog.String("value", model.MaskValue(c.Type, c.Value)),
				slog.String("reason", reason))

			continue
		}

		out = append(out, c)
	}

	return out
}

// unusable says why a criterion cannot be searched; empty when it can.
func unusable(c model.IdentificationCriterion) string {
	if c.Value == "" || utf8.RuneCountInString(c.Value) > maxCriterionValueLen {
		return "empty or too long value"
	}

	switch c.Type {
	case model.CriterionPhone:
		if len(model.PhoneDigits(c.Value)) < minPhoneDigits {
			return "fewer than 4 digits"
		}
	case model.CriterionEmail:
		if t := substringTerm(c.Value); len(t) < minEmailLen || !strings.Contains(t, "@") {
			return "not an email"
		}
	case model.CriterionAttribute:
		if !model.IsSearchableKey(c.Key) {
			return "attribute key is empty, too long or holds a search operator"
		}

		if substringTerm(c.Value) == "" {
			return "no searchable text"
		}
	case model.CriterionUnspecified, model.CriterionMessengerID:
		return "unsupported type"
	default:
		return "unsupported type"
	}

	return ""
}

// criteriaMatches searches the schema's criteria: any of them may match, and
// what a schema sends is never verified.
// A criterion whose search fails is skipped, so the others and the channel
// matches are still stored; only when every criterion failed is the run an
// error (the CRM is down: "found nothing" would mislead the schema).
func (s *ContactIdentificationService) criteriaMatches(ctx context.Context, cmd *model.IdentifyContactCommand) ([]model.ContactMatch, error) {
	var (
		matches  []model.ContactMatch
		failures []error
	)

	for _, c := range cmd.Criteria {
		var (
			found []string
			err   error
		)

		switch c.Type {
		case model.CriterionPhone:
			found, err = s.searchPhone(ctx, cmd.DomainID, c.Value, cmd.PhoneLastDigits, cmd.PhoneScope)
		case model.CriterionEmail:
			found, err = s.searchEmail(ctx, cmd.DomainID, c.Value)
		case model.CriterionAttribute:
			found, err = s.searchAttribute(ctx, cmd.DomainID, c.Key, c.Value)
		case model.CriterionUnspecified, model.CriterionMessengerID:
			continue
		}

		if err != nil {
			s.logger.WarnContext(ctx, "crm search failed",
				slog.String("thread_id", cmd.ThreadID.String()),
				slog.Int("criterion_type", int(c.Type)),
				slog.String("value", model.MaskValue(c.Type, c.Value)),
				slog.Any("err", err))

			failures = append(failures, err)

			continue
		}

		reason := model.MatchReason{Type: c.Type, Trust: model.TrustUnverified, Origin: model.OriginFlow}
		if c.Type == model.CriterionAttribute {
			reason.Key = c.Key
		}

		for _, id := range found {
			matches = append(matches, model.ContactMatch{CRMContactID: id, Reason: reason})
		}
	}

	if len(failures) > 0 && len(failures) == len(cmd.Criteria) {
		return nil, stderrors.Join(failures...)
	}

	return matches, nil
}

// imContactOwners returns the CRM contacts the IM contact is linked to. The
// CRM matches the id exactly, so its answer is taken as is.
func (s *ContactIdentificationService) imContactOwners(ctx context.Context, domainID int, imContactID string) ([]string, error) {
	linked, err := s.search(ctx, model.CRMContactQuery{DomainID: domainID, Field: model.CRMSearchIMContact, Term: imContactID})
	if err != nil {
		return nil, err
	}

	owners := make([]string, 0, len(linked))
	for _, c := range linked {
		owners = append(owners, c.ID)
	}

	return owners, nil
}

// search reads up to crmMaxPages pages of one CRM query.
func (s *ContactIdentificationService) search(ctx context.Context, q model.CRMContactQuery) ([]*model.CRMContact, error) {
	if q.Term == "" {
		// An empty term would match every contact of the domain.
		return nil, nil
	}

	var out []*model.CRMContact

	q.Size = crmPageSize

	for page := int32(1); page <= crmMaxPages; page++ {
		q.Page = page

		contacts, next, err := s.crm.SearchContacts(ctx, q)
		if err != nil {
			return nil, err
		}

		out = append(out, contacts...)

		if !next {
			return out, nil
		}
	}

	// A real match may sit past the last page read; say so instead of hiding it.
	s.logger.WarnContext(ctx, "crm search has more rows than identification reads; narrow the criterion",
		slog.Int("domain_id", q.DomainID), slog.Int("field", int(q.Field)), slog.Int("rows_read", len(out)))

	return out, nil
}

// phoneTerm is what the CRM is asked for: the last N digits when asked, else
// the national tail of the number.
func phoneTerm(digits string, lastN int) string {
	n := phoneSearchTail
	if lastN > 0 {
		n = lastN
	}

	if len(digits) > n {
		return digits[len(digits)-n:]
	}

	return digits
}

// searchChannelPhone splits the CRM contacts holding the channel's number into
// those storing exactly these digits and those storing only their tail.
func (s *ContactIdentificationService) searchChannelPhone(ctx context.Context, domainID int, raw string) (exact, tail []string, err error) {
	digits := model.PhoneDigits(raw)

	contacts, err := s.search(ctx, model.CRMContactQuery{DomainID: domainID, Field: model.CRMSearchPhones, Term: phoneTerm(digits, 0)})
	if err != nil {
		return nil, nil, err
	}

	for _, c := range contacts {
		switch {
		case slices.ContainsFunc(c.Phones, func(p model.CRMPhone) bool { return model.PhoneDigits(p.Number) == digits }):
			exact = append(exact, c.ID)
		case slices.ContainsFunc(c.Phones, func(p model.CRMPhone) bool { return model.PhoneMatches(p.Number, digits, 0) }):
			tail = append(tail, c.ID)
		}
	}

	return exact, tail, nil
}

func (s *ContactIdentificationService) searchPhone(ctx context.Context, domainID int, raw string, lastN int, scope model.PhoneScope) ([]string, error) {
	digits := model.PhoneDigits(raw)
	if digits == "" {
		return nil, nil
	}

	// Digits only: the CRM finds the term inside the stored number as it was
	// typed, so a "+" or a country code in front does not matter, but a number
	// stored with spaces or dashes between the digits is not found; normalising
	// stored numbers is the CRM's concern.
	contacts, err := s.search(ctx, model.CRMContactQuery{DomainID: domainID, Field: model.CRMSearchPhones, Term: phoneTerm(digits, lastN)})
	if err != nil {
		return nil, err
	}

	var ids []string

	for _, c := range contacts {
		for _, p := range c.Phones {
			if scope == model.PhoneScopePrimaryOnly && !p.Primary {
				continue
			}

			if model.PhoneMatches(p.Number, digits, lastN) {
				ids = append(ids, c.ID)

				break
			}
		}
	}

	return ids, nil
}

func (s *ContactIdentificationService) searchEmail(ctx context.Context, domainID int, email string) ([]string, error) {
	contacts, err := s.search(ctx, model.CRMContactQuery{DomainID: domainID, Field: model.CRMSearchEmails, Term: substringTerm(email)})
	if err != nil {
		return nil, err
	}

	var ids []string

	for _, c := range contacts {
		if slices.ContainsFunc(c.Emails, func(e string) bool { return model.EmailMatches(e, email) }) {
			ids = append(ids, c.ID)
		}
	}

	return ids, nil
}

func (s *ContactIdentificationService) searchAttribute(ctx context.Context, domainID int, key, value string) ([]string, error) {
	value = strings.TrimSpace(value)

	term := substringTerm(value)
	if term == "" {
		return nil, nil
	}

	contacts, err := s.search(ctx, model.CRMContactQuery{DomainID: domainID, Field: model.CRMSearchVariables, Term: key + "=" + term})
	if err != nil {
		return nil, err
	}

	var ids []string

	for _, c := range contacts {
		if have, ok := c.Variables[key]; ok && have == value {
			ids = append(ids, c.ID)
		}
	}

	return ids, nil
}

// substringTerm turns a client-entered value into a plain CRM substring
// search. The CRM reads "*" as a wildcard that splits the term, "%" as a LIKE
// wildcard, a leading "=" as exact match and "/x/" as a regular expression;
// all are dropped, so the term is never an operator; so is "\", LIKE's escape.
// The original value is what matches are compared with afterwards.
func substringTerm(v string) string {
	v = strings.NewReplacer("*", "", "%", "", "\\", "").Replace(strings.TrimSpace(v))
	v = strings.TrimLeft(v, "=/")
	v = strings.TrimRight(v, "/")

	return strings.TrimSpace(v)
}

// link adds the IM contact to the CRM contact. It is insert-only: a link made
// to another contact since the caller's check makes the CRM reject it
// (model.ErrIMContactLinked), so a link is never moved.
func (s *ContactIdentificationService) link(ctx context.Context, domainID int, crmContactID string, member *externalMember) error {
	protocol := member.identity.Type
	if protocol == "" {
		protocol = "im"
	}

	return s.crm.LinkIMContact(ctx, model.IMContactLink{
		DomainID:     domainID,
		CRMContactID: crmContactID,
		IMContactID:  member.identity.IMContactID,
		Protocol:     protocol,
		Via:          member.via,
	})
}

// publishUpdated tells the members to refresh the thread and its candidates.
func (s *ContactIdentificationService) publishUpdated(ctx context.Context, uow store.UnitOfWork, threadID uuid.UUID, domainID int, crmContactID string) error {
	members, err := uow.ThreadDialogStore().GetQuickView(ctx, &model.ThreadDialogStoreFilter{ThreadIDs: []uuid.UUID{threadID}, DomainID: domainID})
	if err != nil {
		return err
	}

	participants := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		if m != nil {
			participants = append(participants, m.ContactID)
		}
	}

	updated := &event.ThreadUpdated{
		ThreadID:                 threadID,
		DomainID:                 int32(domainID), //nolint:gosec // G115: domain ids reach the service as int32 proto fields
		CRMContactID:             crmContactID,
		ContactCandidatesChanged: true,
		OccurredAt:               s.now(),
		Participants:             participants,
	}

	return uow.Outbox().Publish(ctx, updated.Topic(), updated)
}

// validateIdentify rejects a malformed call. Single criteria are not judged
// here: an unusable one is skipped (usableCriteria), not an error.
func validateIdentify(cmd *model.IdentifyContactCommand) error {
	const errID = "service.contact_identification.identify"

	if cmd == nil || cmd.ThreadID == uuid.Nil || cmd.DomainID <= 0 {
		return errors.InvalidArgument("thread_id and domain_id are required", errors.WithID(errID))
	}

	if n := cmd.PhoneLastDigits; n != 0 && (n < minPhoneDigits || n > maxChannelPhoneDigits) {
		return errors.InvalidArgument("phone_last_digits must be 0 or between 4 and 15", errors.WithID(errID))
	}

	return nil
}
