package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type referenceStore struct {
	db Querier
}

var _ store.ReferenceStore = (*referenceStore)(nil)

func NewReferenceStore(db Querier) *referenceStore {
	return &referenceStore{db: db}
}

// UpsertSource writes the display data of a provider gate. updated_at changes
// only when the stored values differ, so an unchanged upsert is a no-op write.
func (s *referenceStore) UpsertSource(ctx context.Context, ref *model.SourceRef) error {
	if ref == nil || ref.GateID == "" {
		return errors.InvalidArgument("gate id required", errors.WithID("postgres.reference_store.upsert_source"))
	}

	query := `
		insert into im_thread.source_ref (gate_id, domain_id, name, type)
		values (@GateID, @DomainID, @Name, @Type)
		on conflict (gate_id) do update
			set domain_id = excluded.domain_id, name = excluded.name, type = excluded.type,
				deleted = false, updated_at = now()
		where (source_ref.domain_id, source_ref.name, source_ref.type, source_ref.deleted)
			is distinct from (excluded.domain_id, excluded.name, excluded.type, false)
	`

	_, err := s.db.Exec(ctx, query, pgx.NamedArgs{
		"GateID":   ref.GateID,
		"DomainID": ref.DomainID,
		"Name":     ref.Name,
		"Type":     ref.Type,
	})
	if err != nil {
		return errors.Internal("upserting source ref", errors.WithCause(err), errors.WithID("postgres.reference_store.upsert_source"))
	}

	return nil
}

// UpsertQueue writes the display name of a call_center queue. updated_at changes
// only when the stored values differ.
func (s *referenceStore) UpsertQueue(ctx context.Context, ref *model.QueueRef) error {
	if ref == nil || ref.QueueID <= 0 {
		return errors.InvalidArgument("queue id required", errors.WithID("postgres.reference_store.upsert_queue"))
	}

	query := `
		insert into im_thread.queue_ref (domain_id, queue_id, name)
		values (@DomainID, @QueueID, @Name)
		on conflict (domain_id, queue_id) do update
			set name = excluded.name, deleted = false, updated_at = now()
		where (queue_ref.name, queue_ref.deleted) is distinct from (excluded.name, false)
	`

	_, err := s.db.Exec(ctx, query, pgx.NamedArgs{
		"DomainID": ref.DomainID,
		"QueueID":  ref.QueueID,
		"Name":     ref.Name,
	})
	if err != nil {
		return errors.Internal("upserting queue ref", errors.WithCause(err), errors.WithID("postgres.reference_store.upsert_queue"))
	}

	return nil
}
