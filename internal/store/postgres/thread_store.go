package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/im-thread-service/internal/utils"
)

// [D]ata [A]cess [O]bjects
type (
	threadRecord struct {
		ID          uuid.UUID              `json:"id,omitempty" db:"id"`
		DomainID    int                    `json:"domain_id,omitempty" db:"domain_id"`
		Subject     string                 `json:"subject,omitempty" db:"subject"`
		CreatedAt   time.Time              `json:"created_at" db:"created_at"`
		UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
		Kind        model.ThreadKind       `json:"kind,omitempty" db:"kind"`
		Owner       uuid.UUID              `json:"owner,omitempty" db:"owner"`
		Description string                 `json:"description,omitempty" db:"description"`
		Members     []*threadMemberRecord  `json:"members,omitempty" db:"members"`
		LastMessage *model.Message         `json:"last_msg,omitempty" db:"last_msg"`
		Variables   *model.ThreadVariables `json:"variables,omitempty" db:"variables"`
	}
	threadMemberRecord struct {
		ID       uuid.UUID `json:"id,omitempty" db:"id"`
		MemberID uuid.UUID `json:"member_id,omitempty" db:"member_id"`
		Role     int       `json:"role,omitempty" db:"role"`
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
		return nil, err
	}

	return req, nil
}

func (s *threadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	return collectRows(rows, mapThreadRecordToModel)
}

// mapThreadRecordToModel maps a *threadRecord to a *model.Thread.
// It iterates over the thread members and maps each *threadMemberRecord to a *model.ThreadMember.
// If the thread member record has a direct settings record, it is mapped to a *model.DirectThreadSetting.
// The function then returns the mapped *model.Thread record.
func mapThreadRecordToModel(record *threadRecord) (*model.Thread, error) {
	members := utils.Map(record.Members, func(tmr *threadMemberRecord) *model.ThreadDialog {
		return &model.ThreadDialog{
			BaseModel: shared.BaseModel{
				ID: tmr.ID,
			},
			MemberID:   tmr.MemberID,
			ThreadRole: model.ThreadRole(tmr.Role),
		}
	})

	thread := &model.Thread{
		ID:          record.ID,
		DomainID:    record.DomainID,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
		Kind:        record.Kind,
		Subject:     record.Subject,
		Description: record.Description,
		Members:     members,
		LastMessage: record.LastMessage,
		Variables:   record.Variables,
	}

	return thread, nil
}

func (t *threadStore) ResolveDirect(ctx context.Context, from, to uuid.UUID) (*model.Thread, error) {
	var (
		query = `
		SELECT id, domain_id, kind, subject, description, created_at, updated_at, members.aggregated_members members
		FROM im_thread.thread t
		LEFT JOIN LATERAL (
				SELECT jsonb_agg(
					json_build_object('id', id, 'member_id', member_id, 'role', thread_role)
				) AS aggregated_members
				FROM im_thread.thread_dialog dial
				WHERE dial.thread_id = t.id
		) AS members ON true
		WHERE kind = @Kind

				AND id IN (
	           		SELECT thread_id
	            	FROM im_thread.thread_dialog
	            	WHERE
					(
						(member_id = @FromId and direct_to = @DirectTo)
						 or
						(member_id = @DirectTo and direct_to = @FromId)
					)
		            LIMIT 1
				)
        `
		args = pgx.NamedArgs{
			"FromId":   from,
			"DirectTo": to,
			"Kind":     model.ThreadDirect,
		}
	)

	row, err := t.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	res, err := collectRow(row, mapThreadRecordToModel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return res, nil
}

func (t *threadStore) Delete(ctx context.Context, threadID uuid.UUID) error {
	if threadID == uuid.Nil {
		return errors.New("threadID cannot be nil")
	}

	query := `DELETE FROM im_thread.thread WHERE id = $1`

	cmdTag, err := t.db.Exec(ctx, query, threadID)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("no thread found with id %s", threadID)
	}

	return nil
}
