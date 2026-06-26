package postgres

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/webitel-go-kit/infra/pgw"
)

type MessageHistoryStoreFactory struct {
	poolManager *pgw.PoolManager
	wmlogger    watermill.LoggerAdapter
}

func NewMessageHistoryStoreFactory(pm *pgw.PoolManager, wmlogger watermill.LoggerAdapter) *MessageHistoryStoreFactory {
	return &MessageHistoryStoreFactory{
		poolManager: pm,
		wmlogger:    wmlogger,
	}
}

func (sf *MessageHistoryStoreFactory) NewMessageHistoryStore(ctx context.Context) (store.MessageHistoryStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewMessageHistoryStore(pool), nil
}

type ThreadVariablesStoreFactory struct {
	poolManager *pgw.PoolManager
	wmlogger    watermill.LoggerAdapter
}

func NewThreadVariablesStoreFactory(pm *pgw.PoolManager, wmlogger watermill.LoggerAdapter) *ThreadVariablesStoreFactory {
	return &ThreadVariablesStoreFactory{
		poolManager: pm,
		wmlogger:    wmlogger,
	}
}

func (sf *ThreadVariablesStoreFactory) NewThreadVariablesStore(ctx context.Context) (store.ThreadVariablesStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewThreadVariablesStore(pool), nil
}

type MessageStoreFactory struct {
	poolManager *pgw.PoolManager
	wmlogger    watermill.LoggerAdapter
}

func NewMessageStoreFactory(pm *pgw.PoolManager, wmlogger watermill.LoggerAdapter) *MessageStoreFactory {
	return &MessageStoreFactory{
		poolManager: pm,
		wmlogger:    wmlogger,
	}
}

func (sf *MessageStoreFactory) NewMessageStore(ctx context.Context) (store.MessageStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewMessageStore(pool), nil
}

type OutboxStoreFactory struct {
	poolManager *pgw.PoolManager
	wmlogger    watermill.LoggerAdapter
}

func NewOutboxStoreFactory(pm *pgw.PoolManager, wmlogger watermill.LoggerAdapter) *OutboxStoreFactory {
	return &OutboxStoreFactory{
		poolManager: pm,
		wmlogger:    wmlogger,
	}
}

func (sf *OutboxStoreFactory) NewOutboxStore(ctx context.Context) (store.OutboxStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewOutboxStore(pool, sf.wmlogger), nil
}

type ThreadDialogStoreFactory struct {
	poolManager *pgw.PoolManager
}

func NewThreadDialogStoreFactory(pm *pgw.PoolManager) (*ThreadDialogStoreFactory, error) {
	return &ThreadDialogStoreFactory{
		poolManager: pm,
	}, nil
}

func (sf *ThreadDialogStoreFactory) NewThreadDialogStore(ctx context.Context) (store.ThreadDialogStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewThreadDialogStore(pool), nil
}

type ThreadStoreFactory struct {
	poolManager *pgw.PoolManager
}

func NewThreadStoreFactory(pm *pgw.PoolManager) (*ThreadStoreFactory, error) {
	return &ThreadStoreFactory{
		poolManager: pm,
	}, nil
}

func (sf *ThreadStoreFactory) NewThreadStore(ctx context.Context) (store.ThreadStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewThreadStore(pool), nil
}

type ThreadPermissionStoreFactory struct {
	poolManager *pgw.PoolManager
}

func NewThreadPermissionStoreFactory(pm *pgw.PoolManager) (*ThreadPermissionStoreFactory, error) {
	return &ThreadPermissionStoreFactory{
		poolManager: pm,
	}, nil
}

func (sf *ThreadPermissionStoreFactory) NewThreadPermissionStore(ctx context.Context) (store.ThreadPermissionStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewThreadPermissionStore(pool), nil
}

type BotControlStoreFactory struct {
	poolManager *pgw.PoolManager
}

func NewBotControlStoreFactory(pm *pgw.PoolManager) (*BotControlStoreFactory, error) {
	return &BotControlStoreFactory{
		poolManager: pm,
	}, nil
}

func (sf *BotControlStoreFactory) NewBotControlStore(ctx context.Context) (store.BotControlStore, error) {
	pool, err := sf.poolManager.Primary()
	if err != nil {
		return nil, err
	}
	return NewBotControlStore(pool), nil
}
