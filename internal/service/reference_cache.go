package service

import (
	"context"
	"fmt"
	"time"

	"github.com/webitel/webitel-go-kit/pkg/cache"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

const (
	referenceCacheSize = 4096
	referenceCacheTTL  = 30 * time.Minute
)

type sourceRefValue struct {
	DomainID int
	Name     string
	Type     string
}

// ReferenceCache remembers the last values written to source_ref and queue_ref
// so that identical upserts are skipped. It is process-local and best effort:
// a cache error is treated as a miss. Every method is safe on a nil receiver,
// where a nil cache means "always upsert".
type ReferenceCache struct {
	sources cache.Cache[string, sourceRefValue]
	queues  cache.Cache[string, string]
}

func NewReferenceCache() (*ReferenceCache, error) {
	sources, err := cache.New[string, sourceRefValue]().
		L1(cache.RistrettoConfig{
			MaxCost:     referenceCacheSize,
			NumCounters: referenceCacheSize * 10,
			TTL:         referenceCacheTTL,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create source ref cache: %w", err)
	}

	queues, err := cache.New[string, string]().
		L1(cache.RistrettoConfig{
			MaxCost:     referenceCacheSize,
			NumCounters: referenceCacheSize * 10,
			TTL:         referenceCacheTTL,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create queue ref cache: %w", err)
	}

	return &ReferenceCache{sources: sources, queues: queues}, nil
}

// SourceChanged reports whether ref differs from the last remembered value.
func (c *ReferenceCache) SourceChanged(ctx context.Context, ref *model.SourceRef) bool {
	if c == nil || c.sources == nil || ref == nil {
		return true
	}

	cached, ok, err := c.sources.Get(ctx, ref.GateID)
	if err != nil || !ok {
		return true
	}

	return cached != (sourceRefValue{DomainID: ref.DomainID, Name: ref.Name, Type: ref.Type})
}

// RememberSource stores ref as the last written value. Call it only after the
// transaction that wrote ref has committed.
func (c *ReferenceCache) RememberSource(ctx context.Context, ref *model.SourceRef) {
	if c == nil || c.sources == nil || ref == nil {
		return
	}

	_ = c.sources.Set(ctx, ref.GateID, sourceRefValue{DomainID: ref.DomainID, Name: ref.Name, Type: ref.Type})
}

// QueueChanged reports whether ref differs from the last remembered name.
func (c *ReferenceCache) QueueChanged(ctx context.Context, ref *model.QueueRef) bool {
	if c == nil || c.queues == nil || ref == nil {
		return true
	}

	cached, ok, err := c.queues.Get(ctx, queueCacheKey(ref))
	if err != nil || !ok {
		return true
	}

	return cached != ref.Name
}

// RememberQueue stores ref as the last written value. Call it only after the
// transaction that wrote ref has committed.
func (c *ReferenceCache) RememberQueue(ctx context.Context, ref *model.QueueRef) {
	if c == nil || c.queues == nil || ref == nil {
		return
	}

	_ = c.queues.Set(ctx, queueCacheKey(ref), ref.Name)
}

func queueCacheKey(ref *model.QueueRef) string {
	return fmt.Sprintf("%d:%d", ref.DomainID, ref.QueueID)
}

// sourceRefFromPeers builds the source of a thread from the first peer that
// carries via-info. It returns nil when no peer has a via with a non-empty name.
func sourceRefFromPeers(domainID int, peers ...*shared.Peer) *model.SourceRef {
	for _, p := range peers {
		if p == nil {
			continue
		}

		vi := p.ResolveViaInfo()
		if vi == nil || vi.ID == "" || vi.Name == "" {
			continue
		}

		return &model.SourceRef{GateID: vi.ID, DomainID: domainID, Name: vi.Name, Type: vi.Type}
	}

	return nil
}

// refreshSource keeps source_ref up to date. It runs outside the caller's
// transaction: the gate name is true on its own, so it need not be atomic with
// the thread write. Errors are logged, not returned, because a stale display
// name must not fail a send.
func (t *ThreadManagementService) refreshSource(ctx context.Context, ref *model.SourceRef) {
	if ref == nil || !t.refs.SourceChanged(ctx, ref) {
		return
	}

	if err := t.uow.References().UpsertSource(ctx, ref); err != nil {
		t.log().WarnContext(ctx, "failed to refresh source ref", "gate_id", ref.GateID, "err", err)

		return
	}

	t.refs.RememberSource(ctx, ref)
}

// refreshQueue keeps queue_ref up to date, outside the caller's transaction for
// the same reasons as refreshSource. It is a no-op when the queue is absent or
// has no name, because queue_ref.name is not null.
func (t *ThreadManagementService) refreshQueue(ctx context.Context, domainID int, q *model.QueueInfo) {
	if q == nil || q.ID <= 0 || q.Name == "" {
		return
	}

	ref := &model.QueueRef{DomainID: domainID, QueueID: q.ID, Name: q.Name}
	if !t.refs.QueueChanged(ctx, ref) {
		return
	}

	if err := t.uow.References().UpsertQueue(ctx, ref); err != nil {
		t.log().WarnContext(ctx, "failed to refresh queue ref", "queue_id", q.ID, "err", err)

		return
	}

	t.refs.RememberQueue(ctx, ref)
}

// queueIDOf returns the queue id to store in thread_dialog.queue_id, or nil.
func queueIDOf(q *model.QueueInfo) *int64 {
	if q == nil || q.ID <= 0 {
		return nil
	}

	id := q.ID

	return &id
}
