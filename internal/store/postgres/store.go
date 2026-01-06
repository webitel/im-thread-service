package postgres

import (
	"context"

	"github.com/webitel/im-thread-service/internal/store"
)

type Store struct {
	ms store.MessageStore
	os store.OutboxStore
}

func NewStore(ms store.MessageStore, os store.OutboxStore) store.Store {
	return &Store{
		ms: ms,
		os: os,
	}
}

func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if txStore, ok := s.ms.(interface {
		WithTx(context.Context, func(context.Context) error) error
	}); ok {
		return txStore.WithTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Store) Messages() store.MessageStore {
	return s.ms
}

func (s *Store) Outbox() store.OutboxStore {
	return s.os
}
