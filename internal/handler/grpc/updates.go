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
	ChangesSince(ctx context.Context, threadID string, after, horizon int64, limit int) ([]journal.Event, error)
	SettledHorizon(ctx context.Context) (int64, error)
	TrimHorizon(ctx context.Context) (int64, error)
	ReadStates(ctx context.Context, threadID string) ([]journal.ReadState, error)
	IsMember(ctx context.Context, threadID, memberID string, domainID int32) (bool, error)
	Members(ctx context.Context, threadID string) ([]journal.Member, error)
	Unread(ctx context.Context, threadID, contactID string) (int64, error)
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
	out     *mapper.ThreadOutConverter
}

func NewUpdatesServer(history MessageHistoryService, updates UpdatesReader, threads ThreadManagementService) *UpdatesServer {
	return &UpdatesServer{history: history, updates: updates, threads: threads, out: &mapper.ThreadOutConverter{}}
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
		return s.resync(ctx, journal.ResyncFirstSync)
	}

	changes, err := s.updates.ContactChanges(ctx, caller.contactID, req.GetCursor())
	if err != nil {
		if stderrors.Is(err, journal.ErrInvalidCursor) {
			return nil, errors.InvalidArgument("invalid cursor", errors.WithCause(err), errors.WithID("grpc.updates.cursor"))
		}

		return nil, err
	}

	// Retention already removed changes after this cursor: they cannot be replayed.
	trimmed, err := s.updates.TrimHorizon(ctx)
	if err != nil {
		return nil, err
	}

	if changes.After < trimmed {
		return s.resync(ctx, journal.ResyncTrimmed)
	}

	if changes.TooMany {
		return s.resync(ctx, journal.ResyncTooMany)
	}

	resp := &impb.GetUpdatesResponse{Cursor: changes.Cursor}
	budget := journal.MaxContactChanges

	for _, threadID := range changes.Threads {
		entry, used, err := s.threadUpdates(ctx, caller, threadID, changes, budget)
		if err != nil {
			return nil, err
		}

		if used > budget {
			return s.resync(ctx, journal.ResyncTooMany)
		}

		budget -= used

		if entry != nil {
			resp.Threads = append(resp.Threads, entry)
		}
	}

	return resp, nil
}

// resync tells the client to reload its thread list and continue from the returned cursor.
func (s *UpdatesServer) resync(ctx context.Context, reason string) (*impb.GetUpdatesResponse, error) {
	journal.CountResync(ctx, reason)

	horizon, err := s.updates.SettledHorizon(ctx)
	if err != nil {
		return nil, err
	}

	return &impb.GetUpdatesResponse{Cursor: formatSeq(horizon), Resync: true}, nil
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

	events, err := s.updates.ChangesSince(ctx, threadID, window.After, window.Horizon, budget)
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

	referenced := make(map[string]struct{})

	if err := s.applyDiff(ctx, caller, entry, events, referenced); err != nil {
		return nil, 0, err
	}

	if isNewToCaller(events, caller.contactID) {
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

	return entry, len(events), nil
}

// isNewToCaller: the window holds the thread's first change or the caller joining it, so the
// client has never seen this thread.
func isNewToCaller(events []journal.Event, contactID string) bool {
	for _, e := range events {
		if e.Kind == journal.KindThreadCreated {
			return true
		}

		if e.Kind == journal.KindMemberChanged && e.Fields[journal.FieldAction] == journal.ActionJoined && e.Fields[journal.FieldContactID] == contactID {
			return true
		}
	}

	return false
}

// applyDiff folds journal entries into the entry: live messages once in their current
// state, deletions as ids, member changes in order.
func (s *UpdatesServer) applyDiff(ctx context.Context, caller updatesCaller, entry *impb.ThreadUpdates, events []journal.Event, referenced map[string]struct{}) error {
	messages, historyFrom, err := s.currentMessages(ctx, caller, entry.GetThreadId(), events)
	if err != nil {
		return err
	}

	for _, m := range messages {
		if m.GetDeleted() {
			entry.DeletedMessageIds = append(entry.DeletedMessageIds, m.GetId())

			continue
		}

		entry.Messages = append(entry.Messages, toUpdatedMessage(m))
		referenced[m.GetSenderId()] = struct{}{}
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

func formatSeq(seq int64) string { return strconv.FormatInt(seq, 10) }
