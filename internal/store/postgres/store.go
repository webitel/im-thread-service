package postgres

import (
	"github.com/webitel/im-thread-service/internal/store"
)

type Store struct {
	ms store.MessageStore
	os store.OutboxStore
	td store.ThreadDialogStore
	t  store.ThreadStore
	bc store.BotControlStore
}

func NewStore(ms store.MessageStore, os store.OutboxStore, td store.ThreadDialogStore, t store.ThreadStore, bc store.BotControlStore) store.Store {
	return &Store{
		ms: ms,
		os: os,
		td: td,
		t:  t,
		bc: bc,
	}
}

func (s *Store) Messages() store.MessageStore     { return s.ms }
func (s *Store) Outbox() store.OutboxStore        { return s.os }
func (s *Store) ThreadDialog() store.ThreadDialogStore { return s.td }
func (s *Store) Thread() store.ThreadStore        { return s.t }
func (s *Store) BotControl() store.BotControlStore { return s.bc }
