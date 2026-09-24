package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/im-thread-service/internal/utils"
)

// [D]ata [A]cess [O]bjects
type (
	threadRecord struct {
		ID              uuid.UUID              `json:"id,omitempty" db:"id"`
		DomainID        int                    `json:"domain_id,omitempty" db:"domain_id"`
		Subject         string                 `json:"subject,omitempty" db:"subject"`
		CreatedAt       time.Time              `json:"created_at" db:"created_at"`
		UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
		Kind            model.ThreadKind       `json:"kind,omitempty" db:"kind"`
		Owner           uuid.UUID              `json:"owner,omitempty" db:"owner"`
		Description     string                 `json:"description,omitempty" db:"description"`
		Members         []*threadMemberRecord  `json:"members,omitempty" db:"members"`
		LastMessage     *model.Message         `json:"last_msg,omitempty" db:"last_msg"`
		Variables       *model.ThreadVariables `json:"variables,omitempty" db:"variables"`
		BotControllerID *uuid.UUID             `json:"bot_controller_id,omitempty" db:"bot_controller_id"`
		OwnerBotID      *uuid.UUID             `json:"owner_bot_id,omitempty" db:"owner_bot_id"`
		// The real thread id for SearchLeft rows, where "id" above is a
		// membership-period id instead. Absent (zero) from Search's SQL.
		ThreadRefID uuid.UUID `json:"-" db:"thread_ref_id"`
	}
	threadMemberRecord struct {
		ID        uuid.UUID `json:"id,omitempty" db:"id"`
		MemberID  uuid.UUID `json:"member_id,omitempty" db:"member_id"`
		Role      int       `json:"role,omitempty" db:"role"`
		Via       *string   `json:"via,omitempty" db:"via"`
		IsBot     bool      `json:"is_bot" db:"is_bot"`
		AutoLeave bool      `json:"auto_leave" db:"auto_leave"`
	}
)

type threadStore struct {
	db Querier
}

func NewThreadStore(db Querier) *threadStore {
	return &threadStore{
		db: db,
	}
}

func (s *threadStore) Create(ctx context.Context, req *model.Thread) (*model.Thread, error) {
	var (
		query = `
			insert into im_thread.thread (
				domain_id, created_at, updated_at,
				kind, owner, subject, description
			)
			values (@DomainId, @CreatedAt,
				@UpdatedAt, @Kind, @Owner, @Subject,
				@Description
			)
			returning id;
		`
		args = pgx.NamedArgs{
			"DomainId":    req.DomainID,
			"CreatedAt":   req.CreatedAt,
			"UpdatedAt":   req.UpdatedAt,
			"Kind":        req.Kind,
			"Owner":       req.Owner,
			"Subject":     req.Subject,
			"Description": req.Description,
		}
	)

	if err := s.db.QueryRow(ctx, query, args).Scan(&req.ID); err != nil {
		return nil, errors.Internal("creating thread", errors.WithCause(err), errors.WithID("postgres.thread_store.create"), errors.WithValue("query", queryobject.CompactSQL(query)))
	}

	return req, nil
}

func (s *threadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	sql, args, err := query.ToSQL()
	if err != nil {
		return nil, errors.Internal("preparing search thread query", errors.WithCause(err), errors.WithID("postgres.thread_store.search"))
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.NotFound("zero records found for given filters", errors.WithCause(err), errors.WithID("postgres.thread_store.search"))
		}

		return nil, errors.Internal("executing search thread query", errors.WithCause(err), errors.WithID("postgres.thread_store.search"), errors.WithValue("query", sql))
	}

	records, err := collectRows(rows, mapThreadRecordToModel)
	if err != nil {
		return nil, errors.Internal("collecting thread records", errors.WithCause(err), errors.WithID("postgres.thread_store.search"))
	}

	return records, nil
}

func (s *threadStore) Get(ctx context.Context, query queryobject.QueryObject) (*model.Thread, error) {
	sql, args, err := query.ToSQL()
	if err != nil {
		return nil, errors.Internal("preparing get thread query", errors.WithCause(err), errors.WithID("postgres.thread_store.get"))
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Internal("executing get thread query", errors.WithCause(err), errors.WithID("postgres.thread_store.get"), errors.WithValue("query", sql))
	}

	record, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[threadRecord])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.NotFound("thread not found", errors.WithID("postgres.thread_store.get"))
		}

		return nil, errors.Internal("collecting get thread record", errors.WithCause(err), errors.WithID("postgres.thread_store.get"))
	}

	return mapThreadRecordToModel(record)
}

func mapThreadRecordToModel(record *threadRecord) (*model.Thread, error) {
	members := utils.Map(record.Members, func(tmr *threadMemberRecord) *model.ThreadDialog {
		return &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: tmr.ID,
			},
			ContactID:  tmr.MemberID,
			ThreadRole: model.ThreadRole(tmr.Role),
			Via:        tmr.Via,
			IsBot:      tmr.IsBot,
			AutoLeave:  tmr.AutoLeave,
		}
	})

	tagLookupID := record.ID
	if record.ThreadRefID != uuid.Nil {
		tagLookupID = record.ThreadRefID
	}

	thread := &model.Thread{
		ID:              record.ID,
		TagLookupID:     tagLookupID,
		DomainID:        record.DomainID,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		Kind:            record.Kind,
		Subject:         record.Subject,
		Description:     record.Description,
		Members:         members,
		LastMessage:     record.LastMessage,
		Variables:       record.Variables,
		BotControllerID: record.BotControllerID,
		OwnerBotID:      record.OwnerBotID,
	}

	return thread, nil
}

func (s *threadStore) ResolveThread(ctx context.Context, q model.ResolveThreadQuery) (*model.Thread, error) {
	query, args := prepareResolveThreadQuery(q)

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"query resolve thread",
			errors.WithCause(err),
			errors.WithID("postgres.thread_store.resolve_thread"),
			errors.WithValue("query", queryobject.CompactSQL(query)),
		)
	}

	thread, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Thread])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if q.To.Type == shared.PeerContact && q.SendAsPtr() == nil { // in case of contact thread no rows means that it`s first message for contact
				return nil, nil //nolint:nilnil
			}

			return nil, errors.Forbidden(
				"member isn`t part of the thread",
				errors.WithCause(err),
				errors.WithID("postgres.thread_store.resolve_thread"),
				errors.WithValue("thread_id", q.To.ResolveThreadID()),
				errors.WithValue("member_id", q.From.ResolveContactID()),
			)
		}

		return nil, errors.Internal("collect thread", errors.WithCause(err), errors.WithID("postgres.thread_store.resolve_thread"))
	}

	return thread, nil
}

func prepareResolveThreadQuery(q model.ResolveThreadQuery) (string, pgx.NamedArgs) {
	query := `
		   	select
				t.id as id,
				t.domain_id as domain_id,
				t.kind as kind,
				t.subject as subject,
				t.description as description,
				t.created_at as created_at,
				t.updated_at as updated_at,
				t.owner as owner,
				t.bot_controller_id as bot_controller_id,
				m.members as members
			from im_thread.thread t
			inner join im_thread.thread_dialog td_main on td_main.thread_id = t.id
			cross join lateral (
				select jsonb_agg(
					jsonb_build_object(
						'id', td3.id,
						'member_role', td3.thread_role,
						'member_id', td3.member_id,
						'via', td3.via,
						'is_bot', td3.is_bot,
						'auto_leave', td3.auto_leave
					)
				) as members
				from im_thread.thread_dialog td3
				where td3.thread_id = t.id and td3.deleted_at is null
			) m
			where
				(@ToThreadID::uuid is null or (t.id = @ToThreadID::uuid and td_main.deleted_at is null))
				and (
					@ToPeerID::uuid is null
					or (
						t.kind = 1
						and td_main.member_id = @ToPeerID::uuid
						and td_main.deleted_at is null
					)
				)
				and exists (
					select 1 from im_thread.thread_dialog td_check
					where td_check.thread_id = t.id
					  and td_check.member_id = coalesce(@SendAs::uuid, @FromID::uuid)
					  and td_check.deleted_at is null
					  and (@FromVia::text is null or td_check.via = @FromVia::text)
				)
			limit 1;
	`

	args := pgx.NamedArgs{
		"FromID":     q.From.ResolveContactID(),
		"FromVia":    q.From.ResolveVia(),
		"ToPeerID":   q.To.ResolveContactID(),
		"ToThreadID": q.To.ResolveThreadID(),
		"SendAs":     q.SendAsPtr(),
	}

	return query, args
}

func (s *threadStore) SearchLeft(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	sql, args, err := query.ToSQL()
	if err != nil {
		return nil, errors.Internal("preparing search left query", errors.WithCause(err), errors.WithID("postgres.thread_store.search_left"))
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Internal("executing search left query", errors.WithCause(err), errors.WithID("postgres.thread_store.search_left"))
	}

	records, err := collectRows(rows, mapThreadRecordToModel)
	if err != nil {
		return nil, errors.Internal("collecting search left records", errors.WithCause(err), errors.WithID("postgres.thread_store.search_left"))
	}

	return records, nil
}

// TODO: rewrite to soft delete logic!
func (s *threadStore) Delete(ctx context.Context, threadID uuid.UUID) error {
	if threadID == uuid.Nil {
		return errors.InvalidArgument("thread id is required", errors.WithID("postgres.thread_store.delete"))
	}

	query := `DELETE FROM im_thread.thread WHERE id = $1`

	cmdTag, err := s.db.Exec(ctx, query, threadID)
	if err != nil {
		return errors.Internal("executing delete thread query", errors.WithCause(err), errors.WithID("postgres.thread_store.delete"), errors.WithValue("thread_id", threadID.String()))
	}

	if cmdTag.RowsAffected() == 0 {
		return errors.NotFound("zero rows affected", errors.WithID("postgres.thread_store.delete"))
	}

	return nil
}
