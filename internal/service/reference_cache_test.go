package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// mapCache is a synchronous in-memory cache.Cache. Ristretto writes are async,
// so the reference cache tests use this fake instead.
type mapCache[K comparable, V any] struct {
	m map[K]V
}

func newMapCache[K comparable, V any]() *mapCache[K, V] {
	return &mapCache[K, V]{m: map[K]V{}}
}

func (c *mapCache[K, V]) Get(_ context.Context, key K) (V, bool, error) {
	v, ok := c.m[key]

	return v, ok, nil
}

func (c *mapCache[K, V]) Set(_ context.Context, key K, value V) error {
	c.m[key] = value

	return nil
}

func (c *mapCache[K, V]) SetTTL(ctx context.Context, key K, value V, _ time.Duration) error {
	return c.Set(ctx, key, value)
}

func (c *mapCache[K, V]) Delete(_ context.Context, key K) error {
	delete(c.m, key)

	return nil
}

func (c *mapCache[K, V]) Close() error { return nil }

func newTestReferenceCache() *ReferenceCache {
	return &ReferenceCache{
		sources: newMapCache[string, sourceRefValue](),
		queues:  newMapCache[string, string](),
	}
}

func viaPeer(info *shared.ViaInfo) *shared.Peer {
	return &shared.Peer{
		Type:     shared.PeerContact,
		Identity: &shared.Identity{Name: "customer", ViaInfo: info},
	}
}

func TestSourceRefFromPeers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		peers []*shared.Peer
		want  *model.SourceRef
	}{
		{
			name:  "no via info",
			peers: []*shared.Peer{viaPeer(nil)},
		},
		{
			name:  "gate id only (old provider)",
			peers: []*shared.Peer{viaPeer(&shared.ViaInfo{ID: "gate-1"})},
		},
		{
			name:  "via info with empty name",
			peers: []*shared.Peer{viaPeer(&shared.ViaInfo{ID: "gate-1", Type: "telegram"})},
		},
		{
			name:  "no gate id",
			peers: []*shared.Peer{{Identity: &shared.Identity{Name: "x", ViaInfo: &shared.ViaInfo{Name: "Bot"}}}},
		},
		{
			name:  "nil peers are skipped",
			peers: []*shared.Peer{nil, viaPeer(&shared.ViaInfo{ID: "gate-2", Name: "Support", Type: "telegram_bot"})},
			want:  &model.SourceRef{GateID: "gate-2", DomainID: 7, Name: "Support", Type: "telegram_bot"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sourceRefFromPeers(7, tt.peers...)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReferenceCache_NilIsAlwaysChanged(t *testing.T) {
	t.Parallel()

	var refs *ReferenceCache

	ref := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "n", Type: "t"}

	require.True(t, refs.SourceChanged(context.Background(), ref))
	require.True(t, refs.QueueChanged(context.Background(), &model.QueueRef{DomainID: 1, QueueID: 2, Name: "q"}))
	require.NotPanics(t, func() { refs.RememberSource(context.Background(), ref) })
}

func TestReferenceCache_SkipsUnchangedSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refs := newTestReferenceCache()
	ref := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "Support", Type: "telegram"}

	require.True(t, refs.SourceChanged(ctx, ref))

	refs.RememberSource(ctx, ref)
	require.False(t, refs.SourceChanged(ctx, ref))

	renamed := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "Sales", Type: "telegram"}
	require.True(t, refs.SourceChanged(ctx, renamed))
}

func TestReferenceCache_SkipsUnchangedQueue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refs := newTestReferenceCache()
	ref := &model.QueueRef{DomainID: 1, QueueID: 7, Name: "Sales"}

	require.True(t, refs.QueueChanged(ctx, ref))

	refs.RememberQueue(ctx, ref)
	require.False(t, refs.QueueChanged(ctx, ref))

	// The same queue id in another domain is a different row.
	require.True(t, refs.QueueChanged(ctx, &model.QueueRef{DomainID: 2, QueueID: 7, Name: "Sales"}))
	require.True(t, refs.QueueChanged(ctx, &model.QueueRef{DomainID: 1, QueueID: 7, Name: "Renamed"}))
}

func TestRefreshSource_NilCacheAlwaysUpserts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	svc := &ThreadManagementService{uow: fakeUnitOfWork{referenceStore: refStore}}
	ref := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "Support", Type: "telegram"}

	svc.refreshSource(ctx, ref)
	svc.refreshSource(ctx, ref)

	require.Len(t, refStore.sources, 2)
}

func TestRefreshSource_CachedValueSkipsUpsert(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	svc := &ThreadManagementService{uow: fakeUnitOfWork{referenceStore: refStore}, refs: newTestReferenceCache()}
	ref := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "Support", Type: "telegram"}

	svc.refreshSource(ctx, ref)
	svc.refreshSource(ctx, ref)
	require.Len(t, refStore.sources, 1)

	renamed := *ref
	renamed.Name = "Sales"
	svc.refreshSource(ctx, &renamed)
	require.Len(t, refStore.sources, 2)
}

func TestRefreshSource_ErrorIsLoggedAndNotRemembered(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refs := newTestReferenceCache()
	svc := &ThreadManagementService{uow: fakeUnitOfWork{referenceStore: &fakeReferenceStore{err: context.DeadlineExceeded}}, refs: refs}
	ref := &model.SourceRef{GateID: "gate", DomainID: 1, Name: "Support", Type: "telegram"}

	svc.refreshSource(ctx, ref)

	require.True(t, refs.SourceChanged(ctx, ref), "a failed upsert must not be remembered")
}

func TestRefreshQueue_SkipsAbsentOrUnnamedQueue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	svc := &ThreadManagementService{uow: fakeUnitOfWork{referenceStore: refStore}, refs: newTestReferenceCache()}

	for _, q := range []*model.QueueInfo{nil, {ID: 0, Name: "Sales"}, {ID: 5, Name: ""}} {
		svc.refreshQueue(ctx, 1, q)
	}

	require.Empty(t, refStore.queues)
}

func TestRefreshQueue_CachedValueSkipsUpsert(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refStore := &fakeReferenceStore{}
	svc := &ThreadManagementService{uow: fakeUnitOfWork{referenceStore: refStore}, refs: newTestReferenceCache()}
	q := &model.QueueInfo{ID: 7, Name: "Sales"}

	svc.refreshQueue(ctx, 1, q)
	svc.refreshQueue(ctx, 1, q)

	require.Len(t, refStore.queues, 1)
}

func TestQueueIDOf(t *testing.T) {
	t.Parallel()

	require.Nil(t, queueIDOf(nil))
	require.Nil(t, queueIDOf(&model.QueueInfo{ID: 0, Name: "Sales"}))

	id := queueIDOf(&model.QueueInfo{ID: 7, Name: "Sales"})
	require.NotNil(t, id)
	require.Equal(t, int64(7), *id)
}
