package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/im-thread-service/internal/utils"
)

var (
	threadFields = []string{
		"id", "domain_id", "created_by", "created_at", "updated_by", "updated_at",
		"kind", "owner", "subject", "description",
	}
)

// [D]ata [A]cess [O]bjects
type (
	threadRecord struct {
		ID          uuid.UUID             `json:"id,omitempty" db:"id"`
		DomainID    int                   `json:"domain_id,omitempty" db:"domain_id"`
		Subject     string                `json:"subject,omitempty" db:"subject"`
		CreatedAt   time.Time             `json:"created_at,omitempty" db:"created_at"`
		UpdatedAt   time.Time             `json:"updated_at,omitempty" db:"updated_at"`
		Kind        model.ThreadKind      `json:"kind,omitempty" db:"kind"`
		Owner       uuid.UUID             `json:"owner,omitempty" db:"owner"`
		Description string                `json:"description,omitempty" db:"description"`
		MemberIDs   uuid.UUIDs            `json:"member_ids,omitempty" db:"member_ids"`
		Members     []*threadMemberRecord `json:"members,omitempty" db:"members"`
	}
	threadMemberRecord struct {
		ID             uuid.UUID                   `json:"id,omitempty" db:"id"`
		DirectSettings *directThreadSettingsRecord `json:"direct_settings,omitempty" db:"direct_settings"`
	}

	directThreadSettingsRecord struct {
		ID        uuid.UUID `json:"id,omitempty" db:"id"`
		Title     string    `json:"title,omitempty" db:"title"`
		DomainID  int       `json:"domain_id,omitempty" db:"domain_id"`
		CreatedAt time.Time `json:"created_at,omitempty" db:"created_at"`
		UpdatedAt time.Time `json:"updated_at,omitempty" db:"updated_at"`
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

// Create creates a new thread with the given request.
// It returns the newly created thread or an error if the operation fails.
// If the operation succeeds, it returns the newly created thread with the id set.
// If the operation fails, it returns nil and the error.
func (t *threadStore) Create(ctx context.Context, req *model.Thread) (*model.Thread, error) {
	var (
		query = `
			insert into im_thread.thread (
				domain_id, created_by, created_at, updated_by, updated_at, 
				kind, owner, subject, description
			)
			values (@DomainId, @CreatedBy, @CreatedAt, @UpdatedBy, 
				@UpdatedAt, @Kind, @Owner, @Subject,
				@Description
			)
			returning id;
		`
		args = pgx.NamedArgs{
			"DomainId":    req.DomainID,
			"CreatedBy":   req.CreatedBy,
			"CreatedAt":   req.CreatedAt,
			"UpdatedBy":   req.UpdatedBy,
			"UpdatedAt":   req.UpdatedAt,
			"Kind":        req.Kind,
			"Owner":       req.Owner,
			"Subject":     req.Subject,
			"Description": req.Description,
		}
	)

	// SCAN ONLY ID AS OTHER PARAMS ALREADY HAS BEEN SET ON APP LEVEL
	if err := t.db.QueryRow(ctx, query, args).Scan(&req.ID); err != nil {
		return nil, err
	}

	return req, nil
}

// Search searches for threads based on the given request.
// It returns the threads found by the search, or an error if the operation fails.
// If the operation succeeds, it returns the threads found by the search.
// If the operation fails, it returns nil and the error.
func (t *threadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := t.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	records, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[threadRecord])
	if err != nil {
		return nil, err
	}

	threads := utils.Map(records, mapThreadRecordToModel)

	return threads, nil
}

// mapThreadRecordToModel maps a *threadRecord to a *model.Thread.
// It iterates over the thread members and maps each *threadMemberRecord to a *model.ThreadMember.
// If the thread member record has a direct settings record, it is mapped to a *model.DirectThreadSetting.
// The function then returns the mapped *model.Thread record.
func mapThreadRecordToModel(record *threadRecord) *model.Thread {
	members := utils.Map(record.Members, func(tmr *threadMemberRecord) *model.ThreadMember {
		var (
			directSettings *model.DirectThreadSetting
		)

		if tmr.DirectSettings != nil {
			directSettings = &model.DirectThreadSetting{
				BaseThreadSetting: model.BaseThreadSetting{
					BaseModel: shared.BaseModel{
						ID:        tmr.DirectSettings.ID,
						DomainID:  tmr.DirectSettings.DomainID,
						CreatedAt: tmr.DirectSettings.CreatedAt,
						UpdatedAt: tmr.DirectSettings.UpdatedAt,
					},
					Title: tmr.DirectSettings.Title,
				},
			}
		}

		return &model.ThreadMember{
			Id:             tmr.ID,
			DirectSettings: directSettings,
		}
	})

	thread := &model.Thread{
		BaseModel: shared.BaseModel{
			ID:        record.ID,
			DomainID:  record.DomainID,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		},
		Kind:        record.Kind,
		Owner:       record.Owner,
		Subject:     record.Subject,
		Description: record.Description,
		Members:     members,
	}

	return thread
}

func (t *threadStore) ResolveDirect(ctx context.Context, from, to uuid.UUID) (*model.Thread, error) {
	var (
		query = fmt.Sprintf(`
		SELECT %s FROM im_thread.thread
				WHERE kind = @Kind

				AND id IN (
	           		select thread_id
	            	from im_thread.thread_dialog
	            	where 
					(
						(member_id = @FromId and direct_to = @DirectTo)
						 or
						(member_id = @DirectTo and direct_to = @FromId)
					)
		            limit 1
				)
        `, strings.Join(threadFields, ","))
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
	records, err := pgx.CollectExactlyOneRow(row, pgx.RowToAddrOfStructByNameLax[threadRecord])
	if err != nil {
		return nil, err
	}
	return mapThreadRecordToModel(records), nil
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
