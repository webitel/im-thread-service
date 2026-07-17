package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

type fakeMessageExternalStore struct {
	saved            []*model.MessageExternalID
	messageIDByExt   map[string]uuid.UUID
	externalIDByMsg  map[uuid.UUID]string
	lookupMessageErr error
}

func (f *fakeMessageExternalStore) Save(ctx context.Context, rec *model.MessageExternalID) error {
	f.saved = append(f.saved, rec)

	return nil
}

func (f *fakeMessageExternalStore) LookupMessageID(ctx context.Context, gateID, externalID string) (uuid.UUID, error) {
	if f.lookupMessageErr != nil {
		return uuid.Nil, f.lookupMessageErr
	}

	return f.messageIDByExt[externalID], nil
}

func (f *fakeMessageExternalStore) LookupExternalID(ctx context.Context, messageID uuid.UUID, gateID string) (string, error) {
	return f.externalIDByMsg[messageID], nil
}

func newReplyTestService(messageStore *fakeMessageStore, externalStore *fakeMessageExternalStore) *MessageService {
	return &MessageService{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		uow: fakeUnitOfWork{
			messageStore:         messageStore,
			messageExternalStore: externalStore,
		},
	}
}

func TestResolveReplyPreview_Strict(t *testing.T) {
	threadID := uuid.New()
	targetID := uuid.New()

	tests := []struct {
		name      string
		replyToID uuid.UUID
		preview   *model.ReplyToPreview
		wantErr   bool
	}{
		{
			name:      "uuid.Nil rejected",
			replyToID: uuid.Nil,
			wantErr:   true,
		},
		{
			name:      "target not found",
			replyToID: targetID,
			preview:   nil,
			wantErr:   true,
		},
		{
			name:      "target in another thread",
			replyToID: targetID,
			preview:   &model.ReplyToPreview{MessageID: targetID, ThreadID: uuid.New()},
			wantErr:   true,
		},
		{
			name:      "target resolved",
			replyToID: targetID,
			preview:   &model.ReplyToPreview{MessageID: targetID, ThreadID: threadID, Body: "original"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newReplyTestService(&fakeMessageStore{replyPreview: tt.preview}, nil)

			preview, err := svc.resolveReplyPreview(context.Background(), tt.replyToID, threadID, 1)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, preview)
			require.Equal(t, tt.preview.MessageID, preview.MessageID)
		})
	}
}

func TestResolveReply_LenientExternal(t *testing.T) {
	threadID := uuid.New()
	targetID := uuid.New()
	gateID := uuid.New().String()
	clientContactID := uuid.New()

	thread := &model.Thread{
		ID: threadID,
		Members: []*model.ThreadDialog{
			{ContactID: clientContactID, Via: &gateID},
		},
	}
	from := &shared.Peer{ID: clientContactID}

	t.Run("resolved external reference links the reply", func(t *testing.T) {
		svc := newReplyTestService(
			&fakeMessageStore{replyPreview: &model.ReplyToPreview{MessageID: targetID, ThreadID: threadID}},
			&fakeMessageExternalStore{messageIDByExt: map[string]uuid.UUID{"mid.123": targetID}},
		)

		preview, err := svc.resolveReply(context.Background(), nil, "mid.123", from, thread, 1)
		require.NoError(t, err)
		require.NotNil(t, preview)
		require.Equal(t, targetID, preview.MessageID)
	})

	t.Run("unknown external reference degrades to no link", func(t *testing.T) {
		svc := newReplyTestService(
			&fakeMessageStore{},
			&fakeMessageExternalStore{},
		)

		preview, err := svc.resolveReply(context.Background(), nil, "mid.unknown", from, thread, 1)
		require.NoError(t, err)
		require.Nil(t, preview)
	})

	t.Run("target from another thread degrades to no link", func(t *testing.T) {
		svc := newReplyTestService(
			&fakeMessageStore{replyPreview: &model.ReplyToPreview{MessageID: targetID, ThreadID: uuid.New()}},
			&fakeMessageExternalStore{messageIDByExt: map[string]uuid.UUID{"mid.old": targetID}},
		)

		preview, err := svc.resolveReply(context.Background(), nil, "mid.old", from, thread, 1)
		require.NoError(t, err)
		require.Nil(t, preview)
	})

	t.Run("no gate for peer degrades to no link", func(t *testing.T) {
		svc := newReplyTestService(&fakeMessageStore{}, &fakeMessageExternalStore{})

		strangerFrom := &shared.Peer{ID: uuid.New()}
		preview, err := svc.resolveReply(context.Background(), nil, "mid.123", strangerFrom, thread, 1)
		require.NoError(t, err)
		require.Nil(t, preview)
	})

	t.Run("explicit internal id wins over external reference", func(t *testing.T) {
		svc := newReplyTestService(
			&fakeMessageStore{replyPreview: &model.ReplyToPreview{MessageID: targetID, ThreadID: threadID}},
			&fakeMessageExternalStore{},
		)

		preview, err := svc.resolveReply(context.Background(), &targetID, "mid.ignored", from, thread, 1)
		require.NoError(t, err)
		require.NotNil(t, preview)
		require.Equal(t, targetID, preview.MessageID)
	})
}

func TestGateIDForPeer(t *testing.T) {
	gate := "gate-1"
	contactID := uuid.New()

	t.Run("identity via preferred", func(t *testing.T) {
		peer := &shared.Peer{ID: contactID, Identity: &shared.Identity{Via: &gate}}
		require.Equal(t, gate, gateIDForPeer(peer, nil))
	})

	t.Run("falls back to membership via", func(t *testing.T) {
		peer := &shared.Peer{ID: contactID}
		members := []*model.ThreadDialog{{ContactID: contactID, Via: &gate}}
		require.Equal(t, gate, gateIDForPeer(peer, members))
	})

	t.Run("no via anywhere", func(t *testing.T) {
		peer := &shared.Peer{ID: contactID}
		require.Empty(t, gateIDForPeer(peer, []*model.ThreadDialog{{ContactID: uuid.New()}}))
	})
}

func TestRecordInboundExternalID(t *testing.T) {
	gate := uuid.New().String()
	contactID := uuid.New()
	msg := &model.Message{
		ID:       uuid.New(),
		ThreadID: uuid.New(),
		To:       []*model.ThreadDialog{{ContactID: contactID, Via: &gate}},
	}
	from := &shared.Peer{ID: contactID}

	t.Run("records inbound mapping", func(t *testing.T) {
		external := &fakeMessageExternalStore{}
		svc := newReplyTestService(&fakeMessageStore{}, external)

		err := svc.recordInboundExternalID(context.Background(), fakeUnitOfWork{messageExternalStore: external}, msg, from, "mid.42")
		require.NoError(t, err)
		require.Len(t, external.saved, 1)
		require.Equal(t, msg.ID, external.saved[0].MessageID)
		require.Equal(t, gate, external.saved[0].GateID)
		require.Equal(t, "mid.42", external.saved[0].ExternalID)
		require.Equal(t, model.ExternalDirectionInbound, external.saved[0].Direction)
	})

	t.Run("skips when no external id", func(t *testing.T) {
		external := &fakeMessageExternalStore{}
		svc := newReplyTestService(&fakeMessageStore{}, external)

		err := svc.recordInboundExternalID(context.Background(), fakeUnitOfWork{messageExternalStore: external}, msg, from, "")
		require.NoError(t, err)
		require.Empty(t, external.saved)
	})

	t.Run("skips when no gate", func(t *testing.T) {
		external := &fakeMessageExternalStore{}
		svc := newReplyTestService(&fakeMessageStore{}, external)

		noGateMsg := &model.Message{ID: uuid.New(), ThreadID: uuid.New()}
		err := svc.recordInboundExternalID(context.Background(), fakeUnitOfWork{messageExternalStore: external}, noGateMsg, &shared.Peer{ID: uuid.New()}, "mid.42")
		require.NoError(t, err)
		require.Empty(t, external.saved)
	})
}
