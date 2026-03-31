package postgres

import (
	"context"
	"log/slog"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type threadDialogPermissionStore struct {
	db     Querier
	logger *slog.Logger
}

func NewThreadPermissionStore(db Querier) *threadDialogPermissionStore {
	return &threadDialogPermissionStore{
		db: db,
	}
}

func (t *threadDialogPermissionStore) Get(ctx context.Context, in *model.GetThreadPermissionRequest) (*model.ThreadPermission, error) {
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
		args = map[string]any{
			"ThreadDialogID": in.ThreadDialogID,
			"Offset":         in.Size,
			"Limit":          in.Page,
		}
		res *model.ThreadPermission
	)

	err := pgxscan.Select(ctx, t.db, &res, query, args)
	if err != nil {
		return nil, err
	}

	return res, nil
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
		WHERE thread_dialog_id = @ThreadDialogID RETURNING id, thread_id, thread_dialog_id, can_send_messages, can_add_members, can_remove_members, can_change_members_permissions, can_change_thread_info, created_at, updated_at`
		res model.ThreadPermission
	)

	err := pgxscan.Get(ctx, t.db, &res, query,
		map[string]any{
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
	return &res, nil
}
