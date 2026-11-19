package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

type contactCandidateStore struct {
	db Querier
}

func NewContactCandidateStore(db Querier) *contactCandidateStore {
	return &contactCandidateStore{db: db}
}

func (s *contactCandidateStore) List(ctx context.Context, threadID uuid.UUID, domainID int) (model.ContactCandidates, error) {
	const query = `
		select crm_contact_id, trust, reasons, created_at, updated_at
		from im_thread.thread_contact_candidate
		where thread_id = @ThreadID and domain_id = @DomainID
		order by trust desc, created_at, crm_contact_id`

	rows, err := s.db.Query(ctx, query, pgx.NamedArgs{"ThreadID": threadID, "DomainID": domainID})
	if err != nil {
		return nil, errors.Internal("listing contact candidates", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.list"))
	}
	defer rows.Close()

	var out model.ContactCandidates

	for rows.Next() {
		var (
			c       = &model.ContactCandidate{ThreadID: threadID, DomainID: domainID}
			reasons []byte
		)

		if err := rows.Scan(&c.CRMContactID, &c.Trust, &reasons, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, errors.Internal("scanning contact candidate", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.list"))
		}

		if err := json.Unmarshal(reasons, &c.Reasons); err != nil {
			return nil, errors.Internal("decoding contact candidate reasons", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.list"))
		}

		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Internal("listing contact candidates", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.list"))
	}

	return out, nil
}

// Upsert writes the candidates in one statement: it runs under the thread's
// row lock, so one round trip per candidate would hold that lock longer.
func (s *contactCandidateStore) Upsert(ctx context.Context, candidates model.ContactCandidates) error {
	const (
		errID = "postgres.contact_candidate_store.upsert"
		query = `
		insert into im_thread.thread_contact_candidate
			(thread_id, domain_id, crm_contact_id, trust, reasons, created_at, updated_at)
		select c.thread_id, c.domain_id, c.crm_contact_id, c.trust, c.reasons::jsonb, c.created_at, c.updated_at
		from unnest(
			@ThreadIDs::uuid[], @DomainIDs::bigint[], @CRMContactIDs::text[], @Trusts::smallint[],
			@Reasons::text[], @CreatedAts::timestamptz[], @UpdatedAts::timestamptz[]
		) as c(thread_id, domain_id, crm_contact_id, trust, reasons, created_at, updated_at)
		on conflict (thread_id, crm_contact_id) do update set
			trust = excluded.trust,
			reasons = excluded.reasons,
			updated_at = excluded.updated_at`
	)

	if len(candidates) == 0 {
		return nil
	}

	var (
		threadIDs  = make([]uuid.UUID, 0, len(candidates))
		domainIDs  = make([]int64, 0, len(candidates))
		contactIDs = make([]string, 0, len(candidates))
		trusts     = make([]int16, 0, len(candidates))
		reasons    = make([]string, 0, len(candidates))
		createdAts = make([]time.Time, 0, len(candidates))
		updatedAts = make([]time.Time, 0, len(candidates))
	)

	for _, c := range candidates {
		r, err := json.Marshal(c.Reasons)
		if err != nil {
			return errors.Internal("encoding contact candidate reasons", errors.WithCause(err), errors.WithID(errID))
		}

		threadIDs = append(threadIDs, c.ThreadID)
		domainIDs = append(domainIDs, int64(c.DomainID))
		contactIDs = append(contactIDs, c.CRMContactID)
		trusts = append(trusts, int16(c.Trust)) //nolint:gosec // G115: trust is a 0..2 enum
		reasons = append(reasons, string(r))
		createdAts = append(createdAts, c.CreatedAt)
		updatedAts = append(updatedAts, c.UpdatedAt)
	}

	if _, err := s.db.Exec(ctx, query, pgx.NamedArgs{
		"ThreadIDs":     threadIDs,
		"DomainIDs":     domainIDs,
		"CRMContactIDs": contactIDs,
		"Trusts":        trusts,
		"Reasons":       reasons,
		"CreatedAts":    createdAts,
		"UpdatedAts":    updatedAts,
	}); err != nil {
		return errors.Internal("storing contact candidates", errors.WithCause(err), errors.WithID(errID))
	}

	return nil
}

func (s *contactCandidateStore) SaveDuplicate(ctx context.Context, dup *model.ContactDuplicate) error {
	const (
		errID = "postgres.contact_candidate_store.save_duplicate"
		query = `
		insert into im_thread.thread_contact_duplicate (thread_id, domain_id, contact_ids, reasons)
		values (@ThreadID, @DomainID, @ContactIDs, @Reasons)
		on conflict (thread_id, contact_ids) do nothing`
	)

	if dup == nil {
		return nil
	}

	reasons, err := json.Marshal(dup.Reasons)
	if err != nil {
		return errors.Internal("encoding duplicate reasons", errors.WithCause(err), errors.WithID(errID))
	}

	if _, err := s.db.Exec(ctx, query, pgx.NamedArgs{
		"ThreadID":   dup.ThreadID,
		"DomainID":   dup.DomainID,
		"ContactIDs": dup.ContactIDs,
		"Reasons":    reasons,
	}); err != nil {
		return errors.Internal("storing contact duplicate", errors.WithCause(err), errors.WithID(errID))
	}

	return nil
}

func (s *contactCandidateStore) ChannelCheckedAt(ctx context.Context, threadID uuid.UUID) (time.Time, bool, error) {
	const query = `select channel_checked_at from im_thread.thread_contact_identification where thread_id = $1`

	var at time.Time

	err := s.db.QueryRow(ctx, query, threadID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, errors.Internal("reading contact identification state", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.channel_checked_at"))
	}

	return at, true, nil
}

func (s *contactCandidateStore) MarkChannelChecked(ctx context.Context, threadID uuid.UUID, domainID int, observedAt time.Time) error {
	const query = `
		insert into im_thread.thread_contact_identification (thread_id, domain_id, channel_checked_at)
		values (@ThreadID, @DomainID, @ObservedAt)
		on conflict (thread_id) do update
		set channel_checked_at = greatest(thread_contact_identification.channel_checked_at, excluded.channel_checked_at)`

	if _, err := s.db.Exec(ctx, query, pgx.NamedArgs{"ThreadID": threadID, "DomainID": domainID, "ObservedAt": observedAt}); err != nil {
		return errors.Internal("storing contact identification state", errors.WithCause(err), errors.WithID("postgres.contact_candidate_store.mark_channel_checked"))
	}

	return nil
}
