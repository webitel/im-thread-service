package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type systemMessageStore struct {
	db Querier
}

func NewSystemMessageStore(q Querier) store.SystemMessageStore {
	return &systemMessageStore{db: q}
}

func (s *systemMessageStore) Save(ctx context.Context, msg *model.SystemMessage) (*model.SystemMessage, error) {
	const query = `
		WITH sys_msg AS (
			INSERT INTO im_message.system_messages (domain_id, thread_id, type, body, metadata)
			VALUES (@DomainID, @ThreadID, @Type, @Body, @Metadata)
			RETURNING id, domain_id, thread_id, type, body, metadata, created_at
		),
		msg_ins AS (
			INSERT INTO im_message.messages (id, domain_id, thread_id, sender_id, type, body, metadata)
			SELECT
				s.id,
				s.domain_id,
				s.thread_id,
				'00000000-0000-0000-0000-000000000000'::uuid,
				4,
				s.body,
				COALESCE(s.metadata, '{}'::jsonb) || jsonb_build_object('type', s.type, 'text', s.body)
			FROM sys_msg s
			RETURNING id
		)
		SELECT s.id, s.domain_id, s.thread_id, s.type, s.body, s.metadata, s.created_at
		FROM sys_msg s
	`

	args := pgx.NamedArgs{
		"DomainID": msg.DomainID,
		"ThreadID": msg.ThreadID,
		"Type":     msg.Type,
		"Body":     msg.Body,
		"Metadata": msg.Metadata,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	saved, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.SystemMessage])
	if err != nil {
		return nil, err
	}

	return saved, nil
}
