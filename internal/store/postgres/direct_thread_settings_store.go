package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/im-thread-service/internal/utils"
)

type (
	directSettingsStore struct {
		db Querier
	}
)

func NewDirectSettingsStore(db Querier) *directSettingsStore {
	return &directSettingsStore{
		db: db,
	}
}

// TODO: pass query object as parameter to give possibility to build insert via select from thread_dialog table by thread_id & member_id
func (d *directSettingsStore) Create(ctx context.Context, setting *model.DirectThreadSetting) (*model.DirectThreadSetting, error) {
	query := `
		insert into im_thread.direct_settings (
			domain_id, thread_dialog_id, title 
		)
		values
		(@DomainID, @ThreadDialogID, @Title)
		returning
			id,
			domain_id,
			created_at,
			updated_at,
			thread_dialog_id,
			title
	`

	args := pgx.NamedArgs{
		"DomainID":       setting.DomainID,
		"ThreadDialogID": setting.ThreadDialogID,
		"Title":          setting.Title,
	}

	if err := d.db.QueryRow(ctx, query, args).Scan(
		&setting.ID,
		&setting.DomainID,
		&setting.CreatedAt,
		&setting.UpdatedAt,
		&setting.ThreadDialogID,
		&setting.Title,
	); err != nil {
		return nil, err
	}

	panic("unimplemented")
}

// TODO: implement
func (d *directSettingsStore) Delete(ctx context.Context) error {
	panic("unimplemented")
}

func (d *directSettingsStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.DirectThreadSetting, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := d.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	records, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[directSettingsRecord])
	if err != nil {
		return nil, err
	}

	settings := utils.Map(records, mapDirectSettingRecordToModel)

	return settings, nil
}

func mapDirectSettingRecordToModel(record *directSettingsRecord) *model.DirectThreadSetting {
	return &model.DirectThreadSetting{
		BaseThreadSetting: model.BaseThreadSetting{
			BaseModel: model.BaseModel{
				ID:        record.ID,
				DomainID:  record.DomainID,
				CreatedAt: record.CreatedAt,
				UpdatedAt: record.UpdatedAt,
			},
			ThreadDialogID: record.ThreadDialogID,
			Title:          record.Title,
		},
	}
}

// TODO: implement
func (d *directSettingsStore) Update(ctx context.Context) ([]*model.DirectThreadSetting, error) {
	panic("unimplemented")
}
