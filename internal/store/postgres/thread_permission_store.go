package postgres

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type threadDialogPermissionStore struct {
	db     Querier
	logger *slog.Logger
}

type threadPermission struct {
	ID                          uuid.UUID `db:"id"`
	ThreadID                    uuid.UUID `db:"thread_id"`
	ThreadDialogID              uuid.UUID `db:"thread_dialog_id"`
	CanSendMessages             bool      `db:"can_send_messages"`
	CanAddMembers               bool      `db:"can_add_members"`
	CanRemoveMembers            bool      `db:"can_remove_members"`
	CanChangeMembersPermissions bool      `db:"can_change_members_permissions"`
	CanChangeThreadInfo         bool      `db:"can_change_thread_info"`

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func NewThreadPermissionStore(db Querier) *threadDialogPermissionStore {
	return &threadDialogPermissionStore{
		db: db,
	}
}

func (t *threadDialogPermissionStore) Get(ctx context.Context, in *model.GetThreadPermissionRequest) ([]*model.ThreadPermission, error) {
	if in == nil {
		return nil, errors.InvalidArgument("request must not be nil")
	}
	if in.ThreadDialogID == uuid.Nil {
		return nil, errors.InvalidArgument("thread dialog id cannot be nil")
	}
	var (
		query = `
		SELECT 
			id,
			thread_id,
			thread_dialog_id,
			can_send_messages,
			can_add_members,
			can_remove_members,
			can_change_members_permissions,
			can_change_thread_info,
			created_at,
			updated_at
		FROM im_thread.thread_permission
		WHERE thread_dialog_id= @ThreadDialogID
		OFFSET @Offset LIMIT @Limit
	`
	)

	rows, err := t.db.Query(ctx, query, pgx.NamedArgs{
		"ThreadDialogID": in.ThreadDialogID,
		"Offset":         in.Size,
		"Limit":          in.Page,
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, mapToThreadPermission)
}

func (t *threadDialogPermissionStore) Create(ctx context.Context, in *model.ThreadPermission) (*model.ThreadPermission, error) {
	if in == nil {
		return nil, errors.InvalidArgument("permissions must not be nil")
	}
	if in.ThreadDialogID == uuid.Nil {
		return nil, errors.InvalidArgument("thread dialog id required cannot be nil")
	}
	if in.ThreadID == uuid.Nil {
		return nil, errors.InvalidArgument("thread id required cannot be nil")
	}
	var (
		query = `
		INSERT INTO im_thread.thread_permission (
			thread_id,
			thread_dialog_id,
			can_send_messages,
			can_add_members,
			can_remove_members,
			can_change_members_permissions,
			can_change_thread_info
		) VALUES (
			@ThreadDialogID,
			@CanSendMessages,
			@CanAddMembers,
			@CanRemoveMembers,
			@CanChangeMembersPermissions,
			@CanChangeThreadInfo
		) RETURNING id, thread_id, thread_dialog_id, can_send_messages, can_add_members, can_remove_members, can_change_members_permissions, can_change_thread_info, created_at, updated_at`
	)

	rows, err := t.db.Query(ctx, query, pgx.NamedArgs{
		"ThreadDialogID":              in.ThreadDialogID,
		"CanSendMessages":             in.CanSendMessages,
		"CanAddMembers":               in.CanAddMembers,
		"CanRemoveMembers":            in.CanRemoveMembers,
		"CanChangeMembersPermissions": in.CanChangeMembersPermissions,
		"CanChangeThreadInfo":         in.CanChangeThreadInfo,
	})

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectRow(rows, mapToThreadPermission)
}

func mapToThreadPermission(p *threadPermission) (*model.ThreadPermission, error) {
	if p == nil {
		return nil, errors.New("thread permission flat model required to map")
	}
	return &model.ThreadPermission{
		ThreadPermissions: model.ThreadPermissions{
			CanSendMessages:             p.CanSendMessages,
			CanAddMembers:               p.CanAddMembers,
			CanRemoveMembers:            p.CanRemoveMembers,
			CanChangeMembersPermissions: p.CanChangeMembersPermissions,
			CanChangeThreadInfo:         p.CanChangeThreadInfo,
		},
		ID:             p.ID,
		ThreadID:       p.ThreadID,
		ThreadDialogID: p.ThreadDialogID,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}, nil
}

func (t *threadDialogPermissionStore) Update(ctx context.Context, in *model.UpdateThreadPermissionRequest) (*model.ThreadPermission, error) {
	if in == nil {
		return nil, errors.InvalidArgument("permissions must not be nil")
	}
	if in.Initiator == nil || in.Target == nil {
		return nil, errors.InvalidArgument("initiator and target must be provided")
	}
	if in.Target.ThreadDialogID == uuid.Nil {
		return nil, errors.InvalidArgument("thread dialog id required cannot be nil")
	}
	var (
		query = `
		UPDATE im_thread.thread_permission
		SET can_send_messages = COALESCE(@CanSendMessages, can_send_messages),
			can_add_members = COALESCE(@CanAddMembers, can_add_members),
			can_remove_members = COALESCE(@CanRemoveMembers, can_remove_members),
			can_change_members_permissions = COALESCE(@CanChangeMembersPermissions, can_change_members_permissions),
			can_change_thread_info = COALESCE(@CanChangeThreadInfo, can_change_thread_info),
			updated_at = NOW()
		WHERE thread_dialog_id = @ThreadDialogID
	 RETURNING id, thread_id, thread_dialog_id, can_send_messages, can_add_members, can_remove_members, can_change_members_permissions, can_change_thread_info, created_at, updated_at`
	)

	rows, err := t.db.Query(ctx, query,
		pgx.NamedArgs{
			"ThreadDialogID":              in.Target.ThreadDialogID,
			"CanSendMessages":             in.CanSendMessages,
			"CanAddMembers":               in.CanAddMembers,
			"CanRemoveMembers":            in.CanRemoveMembers,
			"CanChangeMembersPermissions": in.CanChangeMembersPermissions,
			"CanChangeThreadInfo":         in.CanChangeThreadInfo,
		})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectRow(rows, mapToThreadPermission)
}
