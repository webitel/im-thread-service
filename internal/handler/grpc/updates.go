package grpc

import (
	"context"
	stderrors "errors"
	"strconv"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/handler/grpc/mapper"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

// UpdatesReader is the slice of the journal GetUpdates needs.
type UpdatesReader interface {
	ContactChanges(ctx context.Context, contactID, cursor string) (*journal.ContactChanges, error)
	ChangesSince(ctx context.Context, threadID string, window *journal.ContactChanges, limit int) ([]journal.Event, error)
	ContactCursor(ctx context.Context, contactID string) (string, error)
	TrimHorizon(ctx context.Context) (int64, error)
	ReadStates(ctx context.Context, threadID string) ([]journal.ReadState, error)
	IsMember(ctx context.Context, threadID, memberID string, domainID int32) (bool, error)
	Members(ctx context.Context, threadID string) ([]journal.Member, error)
	Unread(ctx context.Context, threadID, contactID string) (int64, error)
}

// TagLister reads a contact's own tags on a thread.
type TagLister interface {
	ListTags(ctx context.Context, threadID, contactID uuid.UUID) ([]string, error)
}

// ThreadSearcher loads threads the way ThreadManagement/Search does.
type ThreadSearcher interface {
	Search(ctx context.Context, req *dto.ThreadSearchRequest) ([]*model.Thread, error)
}

type UpdatesServer struct {
	impb.UnimplementedUpdatesServer

	history MessageHistoryService
	updates UpdatesReader
	threads ThreadSearcher
	tags    TagLister
	out     *mapper.ThreadOutConverter
}

func NewUpdatesServer(history MessageHistoryService, updates UpdatesReader, threads ThreadManagementService, tags TagLister) *UpdatesServer {
	return &UpdatesServer{history: history, updates: updates, threads: threads, tags: tags, out: &mapper.ThreadOutConverter{}}
}

// updatesCaller is who asks and how history visibility applies to them.
type updatesCaller struct {
	contactID string
	domainID  int32
	allow     *impb.SystemMessageAllowList
}

// GetUpdates returns every change in the caller's threads since cursor, one entry per changed
// thread, folded to its final state; threads new to the caller also carry the dialog itself.
func (s *UpdatesServer) GetUpdates(ctx context.Context, req *impb.GetUpdatesRequest) (*impb.GetUpdatesResponse, error) {
	if _, err := uuid.Parse(req.GetCallerId()); err != nil {
		return nil, errors.InvalidArgument("invalid caller_id", errors.WithCause(err), errors.WithID("grpc.updates.caller_id"))
	}

	// Membership is tenant-scoped; without a domain it would match any tenant.
	if req.GetDomainId() <= 0 {
		return nil, errors.InvalidArgument("domain_id is required", errors.WithID("grpc.updates.domain_id"))
	}

	caller := updatesCaller{contactID: req.GetCallerId(), domainID: req.GetDomainId(), allow: req.GetSystemMessageAllowList()}

	if req.GetCursor() == "" {
		return s.resync(ctx, caller.contactID, journal.ResyncFirstSync)
	}

	changes, err := s.updates.ContactChanges(ctx, caller.contactID, req.GetCursor())
	if err != nil {
		if stderrors.Is(err, journal.ErrInvalidCursor) {
			return nil, errors.InvalidArgument("invalid cursor", errors.WithCause(err), errors.WithID("grpc.updates.cursor"))
		}

		return nil, err
	}

	if changes.TooMany {
		return s.resync(ctx, caller.contactID, journal.ResyncTooMany)
	}

	// Retention may have removed changes after an old cursor; nothing changed means nothing lost.
	trimmed, err := s.updates.TrimHorizon(ctx)
	if err != nil {
		return nil, err
	}

	if changes.After < trimmed && len(changes.Threads) > 0 {
		return s.resync(ctx, caller.contactID, journal.ResyncTrimmed)
	}

	resp := &impb.GetUpdatesResponse{Cursor: changes.Cursor}
	budget := journal.MaxContactChanges

	for _, threadID := range changes.Threads {
		entry, used, err := s.threadUpdates(ctx, caller, threadID, changes, budget)
		if err != nil {
			return nil, err
		}

		if used > budget {
			return s.resync(ctx, caller.contactID, journal.ResyncTooMany)
		}

		budget -= used

		if entry != nil {
			resp.Threads = append(resp.Threads, entry)
		}
	}

	return resp, nil
}

// resync tells the client to reload its thread list and continue from the returned cursor.
func (s *UpdatesServer) resync(ctx context.Context, contactID, reason string) (*impb.GetUpdatesResponse, error) {
	journal.CountResync(ctx, reason)

	cursor, err := s.updates.ContactCursor(ctx, contactID)
	if err != nil {
		return nil, err
	}

	return &impb.GetUpdatesResponse{Cursor: cursor, Resync: true}, nil
}

// threadUpdates builds one thread's entry from its changes in the served window and reports
// how many changes it consumed; past budget the caller resyncs.
func (s *UpdatesServer) threadUpdates(ctx context.Context, caller updatesCaller, threadID string, window *journal.ContactChanges, budget int) (*impb.ThreadUpdates, int, error) {
	member, err := s.updates.IsMember(ctx, threadID, caller.contactID, caller.domainID)
	if err != nil {
		return nil, 0, err
	}

	if !member {
		return &impb.ThreadUpdates{ThreadId: threadID, Left: true}, 1, nil
	}

	events, err := s.updates.ChangesSince(ctx, threadID, window, budget)
	if err != nil {
		return nil, 0, err
	}

	if len(events) == 0 {
		return nil, 0, nil
	}

	if len(events) > budget {
		return nil, len(events), nil
	}

	entry := &impb.ThreadUpdates{ThreadId: threadID}

	if entry.UnreadCount, err = s.updates.Unread(ctx, threadID, caller.contactID); err != nil {
		return nil, 0, err
	}

	if hasKind(events, journal.KindTagsChanged) {
		if entry.Tags, err = s.callerTags(ctx, caller, threadID); err != nil {
			return nil, 0, err
		}
	}

	referenced := make(map[string]struct{})

	if err := s.applyDiff(ctx, caller, entry, events, referenced); err != nil {
		return nil, 0, err
	}

	if needsDialog(events, caller.contactID) {
		if err := s.attachDialog(ctx, caller, entry); err != nil {
			return nil, 0, err
		}
	}

	readStates, err := s.updates.ReadStates(ctx, threadID)
	if err != nil {
		return nil, 0, err
	}

	for _, rs := range readStates {
		entry.ReadStates = append(entry.ReadStates, &impb.MemberReadState{
			MemberId:         rs.MemberID,
			DeliveredUpToSeq: rs.DeliveredUpToSeq,
			ReadUpToSeq:      rs.ReadUpToSeq,
		})
		referenced[rs.MemberID] = struct{}{}
	}

	members, err := s.threadMembers(ctx, threadID, referenced)
	if err != nil {
		return nil, 0, err
	}

	entry.Members = mergeMembers(members, entry.GetMembers())
	resolveDeleters(entry)

	return entry, len(events), nil
}

func (s *UpdatesServer) callerTags(ctx context.Context, caller updatesCaller, threadID string) (*impb.ThreadTags, error) {
	thread, err := uuid.Parse(threadID)
	if err != nil {
		return nil, errors.Internal("invalid journal thread_id", errors.WithCause(err), errors.WithID("grpc.updates.tags"))
	}

	contact, err := uuid.Parse(caller.contactID)
	if err != nil {
		return nil, errors.InvalidArgument("invalid caller_id", errors.WithCause(err), errors.WithID("grpc.updates.tags"))
	}

	tags, err := s.tags.ListTags(ctx, thread, contact)
	if err != nil {
		return nil, err
	}

	return &impb.ThreadTags{Items: tags}, nil
}

func hasKind(events []journal.Event, kind string) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}

	return false
}

// needsDialog: the window holds the thread's first change or the caller joining it, so the
// client has never seen this thread, or a change of thread attributes the cached copy lacks.
func needsDialog(events []journal.Event, contactID string) bool {
	for _, e := range events {
		if e.Kind == journal.KindThreadCreated || e.Kind == journal.KindThreadUpdated {
			return true
		}

		if e.Kind == journal.KindMemberChanged && e.Fields[journal.FieldAction] == journal.ActionJoined && e.Fields[journal.FieldContactID] == contactID {
			return true
		}
	}

	return false
}

// applyDiff folds journal entries into the entry: live messages once in their current
// state, deletions with when and by whom, member changes in order.
func (s *UpdatesServer) applyDiff(ctx context.Context, caller updatesCaller, entry *impb.ThreadUpdates, events []journal.Event, referenced map[string]struct{}) error {
	messages, historyFrom, err := s.currentMessages(ctx, caller, entry.GetThreadId(), events)
	if err != nil {
		return err
	}

	replaced := replacedMessages(events)

	for _, m := range messages {
		if m.GetDeleted() {
			entry.DeletedMessageIds = append(entry.DeletedMessageIds, m.GetId())
			entry.DeletedMessages = append(entry.DeletedMessages, &impb.DeletedMessage{
				Id:         m.GetId(),
				DeletedAt:  m.GetDeletedAt(),
				DeletedBy:  m.GetDeletedBy(),
				ReplacedBy: replaced[m.GetId()].GetReplacedBy(),
			})
			referenced[m.GetDeletedBy().GetContactId()] = struct{}{}
			delete(replaced, m.GetId())

			continue
		}

		entry.Messages = append(entry.Messages, toUpdatedMessage(m))
		referenced[m.GetSenderId()] = struct{}{}

		for _, f := range m.GetFailures() {
			referenced[f.GetMemberId()] = struct{}{}
		}
	}

	// History hides a failed send replaced by its retry, so its deletion comes
	// from the journal entry alone, in journal order.
	for _, e := range events {
		d, ok := replaced[e.Fields[journal.FieldMsgID]]
		if !ok {
			continue
		}

		delete(replaced, d.GetId())
		entry.DeletedMessageIds = append(entry.DeletedMessageIds, d.GetId())
		entry.DeletedMessages = append(entry.DeletedMessages, d)
		referenced[d.GetDeletedBy().GetContactId()] = struct{}{}
	}

	for _, e := range events {
		if e.Kind != journal.KindMemberChanged {
			continue
		}

		change := &impb.ThreadMemberChange{
			ContactId: e.Fields[journal.FieldContactID],
			Action:    memberChangeActionToProto(e.Fields[journal.FieldAction]),
			ById:      actorOf(e.Fields),
		}
		entry.MemberChanges = append(entry.MemberChanges, change)
		referenced[change.GetContactId()] = struct{}{}
		referenced[change.GetById()] = struct{}{}
	}

	entry.Members = historyFrom

	return nil
}

// replacedMessages collects the deletions of failed sends replaced by their
// retry, keyed by message id. deleted_by carries only the contact; the caller
// resolves it through the entry's members like any other actor.
func replacedMessages(events []journal.Event) map[string]*impb.DeletedMessage {
	out := make(map[string]*impb.DeletedMessage)

	for _, e := range events {
		replacedBy := e.Fields[journal.FieldReplacedBy]
		if e.Kind != journal.KindMessageDeleted || replacedBy == "" {
			continue
		}

		d := &impb.DeletedMessage{Id: e.Fields[journal.FieldMsgID], ReplacedBy: replacedBy}
		d.DeletedAt, _ = strconv.ParseInt(e.Fields[journal.FieldDeletedAt], 10, 64)

		if actor := e.Fields[journal.FieldActor]; actor != "" {
			d.DeletedBy = &impb.ThreadMember{ContactId: actor}
		}

		out[d.GetId()] = d
	}

	return out
}

// attachDialog adds the thread itself and its top message for a thread new to the caller.
func (s *UpdatesServer) attachDialog(ctx context.Context, caller updatesCaller, entry *impb.ThreadUpdates) error {
	id, err := uuid.Parse(entry.GetThreadId())
	if err != nil {
		return nil //nolint:nilerr // journal thread ids are uuids; a bad one just gets no dialog
	}

	threads, err := s.threads.Search(ctx, &dto.ThreadSearchRequest{
		IDs: uuid.UUIDs{id}, DomainIDs: []int{int(caller.domainID)}, SelfID: uuid.MustParse(caller.contactID), Size: 1, Page: 1,
	})
	if err != nil {
		return err
	}

	if len(threads) > 0 {
		entry.Dialog = s.out.ConvertToThread(threads[0])
		// The preview is top_message and the cursor sits on the response, never on the dialog.
		entry.Dialog.LastMsg = nil
		entry.Dialog.UpdatesCursor = ""
	}

	entry.TopMessage, err = s.topMessage(ctx, caller, entry.GetThreadId())

	return err
}

func (s *UpdatesServer) topMessage(ctx context.Context, caller updatesCaller, threadID string) (*impb.UpdatedMessage, error) {
	in := mapper.UpdatesHistoryInput(threadID, caller.contactID, caller.domainID, caller.allow, nil)

	msgs, _, err := s.history.Search(ctx, in)
	if err != nil || len(msgs) == 0 {
		return nil, err
	}

	items := mapper.MapMessage2SearchMessageHistoryResponse(msgs[:1], in.CallerID).GetItems()
	if len(items) == 0 || items[0].GetDeleted() {
		return nil, nil //nolint:nilnil // a thread without a live top message has no preview
	}

	return toUpdatedMessage(items[0]), nil
}

// historyBatch is the most ids one history lookup takes.
const historyBatch = 100

// currentMessages loads the entries' messages through history itself, so visibility matches
// SearchThreadMessagesHistory; messages hidden from the caller are absent.
func (s *UpdatesServer) currentMessages(ctx context.Context, caller updatesCaller, threadID string, events []journal.Event) ([]*impb.HistoryMessage, []*impb.ThreadMember, error) {
	seen := make(map[string]struct{}, len(events))
	ids := make([]string, 0, len(events))

	for _, e := range events {
		id := e.Fields[journal.FieldMsgID]
		if id == "" {
			continue
		}

		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	byID := make(map[string]*impb.HistoryMessage, len(ids))

	var from []*impb.ThreadMember

	for start := 0; start < len(ids); start += historyBatch {
		batch := ids[start:min(start+historyBatch, len(ids))]
		in := mapper.UpdatesHistoryInput(threadID, caller.contactID, caller.domainID, caller.allow, batch)

		msgs, _, err := s.history.Search(ctx, in)
		if err != nil {
			return nil, nil, err
		}

		for _, m := range mapper.MapMessage2SearchMessageHistoryResponse(msgs, in.CallerID).GetItems() {
			byID[m.GetId()] = m
		}

		from = mergeMembers(from, mapper.GetUniqueFrom(msgs))
	}

	// Keep journal order so the diff reads in the order things happened.
	out := make([]*impb.HistoryMessage, 0, len(byID))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}

	return out, from, nil
}

// threadMembers returns the thread's members (left ones included) among the wanted contacts.
func (s *UpdatesServer) threadMembers(ctx context.Context, threadID string, wanted map[string]struct{}) ([]*impb.ThreadMember, error) {
	delete(wanted, "")

	if len(wanted) == 0 {
		return nil, nil
	}

	members, err := s.updates.Members(ctx, threadID)
	if err != nil {
		return nil, err
	}

	out := make([]*impb.ThreadMember, 0, len(wanted))
	for _, m := range members {
		if _, ok := wanted[m.ContactID]; ok {
			out = append(out, &impb.ThreadMember{
				Id: m.ID, ContactId: m.ContactID, IsBot: m.IsBot,
				Role: new(mapper.ThreadOutConverter).ConvertThreadRole(model.ThreadRole(m.Role)),
			})
		}
	}

	return out, nil
}

// resolveDeleters completes deleted_by that a journal entry gave only as a
// contact with the membership the entry's members carry.
func resolveDeleters(entry *impb.ThreadUpdates) {
	byContact := make(map[string]*impb.ThreadMember, len(entry.GetMembers()))
	for _, m := range entry.GetMembers() {
		byContact[m.GetContactId()] = m
	}

	for _, d := range entry.GetDeletedMessages() {
		by := d.GetDeletedBy()
		if by == nil || by.GetId() != "" {
			continue
		}

		if m, ok := byContact[by.GetContactId()]; ok {
			d.DeletedBy = &impb.ThreadMember{Id: m.GetId(), ContactId: m.GetContactId(), IsBot: m.GetIsBot(), Role: m.GetRole()}
		}
	}
}

// actorFields hold the actor's contact id, per entry kind.
var actorFields = []string{journal.FieldActor, journal.FieldContactID}

func actorOf(fields map[string]string) string {
	for _, key := range actorFields {
		if v := fields[key]; v != "" {
			return v
		}
	}

	return ""
}

// toUpdatedMessage keeps what a client needs to render the bubble.
func toUpdatedMessage(h *impb.HistoryMessage) *impb.UpdatedMessage {
	m := &impb.UpdatedMessage{
		Id:            h.GetId(),
		Seq:           h.GetSeq(),
		SenderId:      h.GetSenderId(),
		Type:          h.GetType(),
		Body:          h.GetBody(),
		Metadata:      h.GetMetadata(),
		CreatedAt:     h.GetCreatedAt(),
		ReplyTo:       h.GetReplyTo(),
		ForwardOrigin: h.GetForwardOrigin(),
		Documents:     h.GetDocuments(),
		Images:        h.GetImages(),
		Location:      h.GetLocation(),
		Contact:       h.GetContact(),
		Interactive:   h.GetInteractive(),
		System:        h.GetSystem(),
		Reactions:     h.GetReactions(),
		Internal:      h.GetInternal(),
		Failures:      h.GetFailures(),
	}

	if h.GetRevisionCount() > 0 {
		m.EditedAt = h.GetUpdatedAt()
	}

	return m
}

func memberChangeActionToProto(action string) impb.ThreadMemberChangeAction {
	switch action {
	case journal.ActionJoined:
		return impb.ThreadMemberChangeAction_THREAD_MEMBER_CHANGE_ACTION_JOINED
	case journal.ActionLeft:
		return impb.ThreadMemberChangeAction_THREAD_MEMBER_CHANGE_ACTION_LEFT
	default:
		return impb.ThreadMemberChangeAction_THREAD_MEMBER_CHANGE_ACTION_UNSPECIFIED
	}
}

// mergeMembers appends members not yet present, keyed by contact id.
func mergeMembers(dst, src []*impb.ThreadMember) []*impb.ThreadMember {
	have := make(map[string]struct{}, len(dst))
	for _, m := range dst {
		have[m.GetContactId()] = struct{}{}
	}

	for _, m := range src {
		if _, ok := have[m.GetContactId()]; !ok {
			have[m.GetContactId()] = struct{}{}
			dst = append(dst, m)
		}
	}

	return dst
}
