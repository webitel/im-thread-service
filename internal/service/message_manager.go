package service

import (
	"context"

	"github.com/webitel/im-thread-service/internal/service/dto"
	"github.com/webitel/im-thread-service/internal/store"
)

// interface guards to assure that "messageManager" implements
// enumerated interfaces!
var (
	_ MessageManager = (*messageManager)(nil)
)

type (
	MessageManager interface {
		ManageSendText(ctx context.Context, sendTextRequest *dto.SendTextRequest) (*dto.SendTextResponse, error)
	}

	messageManager struct {
		messageStore store.MessageStore
	}
)

func NewMessageManager(messageStore store.MessageStore) *messageManager {
	return &messageManager{
		messageStore: messageStore,
	}
}

func (m *messageManager) ManageSendText(ctx context.Context, sendTextRequest *dto.SendTextRequest) (*dto.SendTextResponse, error) {
	//[1] Validate text request!
	//[2] Validate peers and that can be send!
	//[3] Validate permissions!
	//[4] Insert record into database and push broker message within one transaction
	// via 'outbox' patter or builded watermill lib implementation to set 'at least one delivered' broker message!!!
	//[5] Return operation result to user call

	panic("implement me!")
}
