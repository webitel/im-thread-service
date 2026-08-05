package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageExternalStore struct {
	db Querier
}

func NewMessageExternalStore(q Querier) store.MessageExternalStore {
	return &messageExternalStore{db: q}
}

func (s *messageExternalStore) Save(ctx context.Context, rec *model.MessageExternalID) error {
	if rec == nil {
		return errors.InvalidArgument("external id record cannot be nil", errors.WithID("postgres.message_external.save"))
	}

	if rec.MessageID == uuid.Nil || rec.GateID == "" || rec.ExternalID == "" {
		return errors.InvalidArgument("message id, gate id and external id are required", errors.WithID("postgres.message_external.save"))
	}

	const query = `
		insert into im_message.message_external_ids (message_id, thread_id, gate_id, external_id, direction)
		values (@MessageID, @ThreadID, @GateID, @ExternalID, @Direction)
		on conflict do nothing
	`

	args := pgx.NamedArgs{
		"MessageID":  rec.MessageID,
		"ThreadID":   rec.ThreadID,
		"GateID":     rec.GateID,
		"ExternalID": rec.ExternalID,
		"Direction":  rec.Direction,
	}

	if _, err := s.db.Exec(ctx, query, args); err != nil {
		return errors.Internal("saving message external id", errors.WithCause(err), errors.WithID("postgres.message_external.save"))
	}

	return nil
}

func (s *messageExternalStore) UpdateDelivery(ctx context.Context, in *model.MessageDelivery) (*model.MessageExternalID, error) {
	if in == nil || in.GateID == "" || in.ExternalID == "" {
		return nil, errors.InvalidArgument("gate id and external id are required", errors.WithID("postgres.message_external.update_delivery"))
	}
	if !in.Status.Valid() {
		return nil, errors.InvalidArgument("unknown delivery status", errors.WithID("postgres.message_external.update_delivery"))
	}

	const query = `
		update im_message.message_external_ids set
			delivered_at = case
				when @Status in (1, 2) then coalesce(delivered_at, @At)
				else delivered_at end,
			read_at = case
				when @Status = 2 then coalesce(read_at, @At)
				else read_at end,
			failed_at = case
				when @Status = 3 then @At
				else failed_at end,
			failed_reason = case
				when @Status = 3 then nullif(@Reason, '')
				else failed_reason end
		where gate_id = @GateID and external_id = @ExternalID
		returning message_id, thread_id, gate_id, external_id, direction
	`

	args := pgx.NamedArgs{
		"GateID":     in.GateID,
		"ExternalID": in.ExternalID,
		"Status":     int16(in.Status),
		"At":         in.At,
		"Reason":     in.Reason,
	}

	var rec model.MessageExternalID
	err := s.db.QueryRow(ctx, query, args).Scan(
		&rec.MessageID,
		&rec.ThreadID,
		&rec.GateID,
		&rec.ExternalID,
		&rec.Direction,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.NotFound("no message for this gate and external id", errors.WithID("postgres.message_external.update_delivery"))
		}

		return nil, errors.Internal("updating message delivery state", errors.WithCause(err), errors.WithID("postgres.message_external.update_delivery"))
	}

	return &rec, nil
}

func (s *messageExternalStore) LookupMessageID(ctx context.Context, gateID, externalID string) (uuid.UUID, error) {
	const query = `
		select message_id
		from im_message.message_external_ids
		where gate_id = @GateID and external_id = @ExternalID
	`

	args := pgx.NamedArgs{
		"GateID":     gateID,
		"ExternalID": externalID,
	}

	var messageID uuid.UUID
	if err := s.db.QueryRow(ctx, query, args).Scan(&messageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}

		return uuid.Nil, errors.Internal("looking up internal message id", errors.WithCause(err), errors.WithID("postgres.message_external.lookup_message_id"))
	}

	return messageID, nil
}

func (s *messageExternalStore) LookupExternalID(ctx context.Context, messageID uuid.UUID, gateID string) (string, error) {
	const query = `
		select external_id
		from im_message.message_external_ids
		where message_id = @MessageID and gate_id = @GateID
	`

	args := pgx.NamedArgs{
		"MessageID": messageID,
		"GateID":    gateID,
	}

	var externalID string
	if err := s.db.QueryRow(ctx, query, args).Scan(&externalID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", errors.Internal("looking up external message id", errors.WithCause(err), errors.WithID("postgres.message_external.lookup_external_id"))
	}

	return externalID, nil
}
