package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

var threadVarsEntity = sqlbuilder.NewStruct(new(model.ThreadVariables)).For(sqlbuilder.PostgreSQL)

type threadVariablesStore struct {
	db Querier
}

func NewThreadVariablesStore(db Querier) *threadVariablesStore {
	return &threadVariablesStore{db: db}
}

func (s *threadVariablesStore) Set(ctx context.Context, variables *model.SetThreadVariablesCommand) (*model.ThreadVariables, error) {
	sql, args := prepareThreadVariablesSetQuery(variables)

	rows, err := s.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal(
			"error setting thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.set"),
			errors.WithValue("thread_id", variables.Variables.ThreadID),
		)
	}

	vars, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadVariables])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.Forbidden(
				"access variable setted by other user",
				errors.WithCause(err),
				errors.WithID("postgres.thread_variables.set"),
				errors.WithValue("thread_id", variables.Variables.ThreadID),
			)
		}

		return nil, errors.Internal(
			"collect thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.set"),
			errors.WithValue("thread_id", variables.Variables.ThreadID),
		)
	}

	return vars, nil
}

func prepareThreadVariablesSetQuery(variables *model.SetThreadVariablesCommand) (string, pgx.NamedArgs) {
	query := `
		with members_list as (
			select array_agg(distinct member_id) as members
			from "im_thread"."thread_dialog"
			where thread_id = @ThreadID
				and deleted_at is null
		),
		upsert_action as (
			insert into "im_thread"."thread_variables" ("thread_id", "variables")
			select
				@ThreadID,
				(
					select jsonb_object_agg(key, value || jsonb_build_object('set_at', now()))
					from jsonb_each(@Variables::jsonb)
				)
			where exists (
				select 1 from members_list m where @Member = any(m.members)
			)
			on conflict ("thread_id") do update
			set variables = thread_variables.variables || excluded.variables
			where not exists (
				select 1
				from jsonb_each(thread_variables.variables) as t(key, val)
				inner join jsonb_each(excluded.variables) as e(k, v) on t.key = e.k
				where t.val->>'set_by' is not null
					and t.val->>'set_by' <> e.v->>'set_by'
			)
			returning thread_id, variables
		)
		select
			ua.thread_id 	as thread_id,
			ua.variables 	as variables,
			ml.members 		as thread_members
		from upsert_action ua
		cross join members_list ml;
	`

	return query, pgx.NamedArgs{
		"ThreadID":  variables.Variables.ThreadID,
		"Variables": variables.Variables.Variables,
		"Member":    variables.Member,
	}
}

func (s *threadVariablesStore) Search(ctx context.Context, query model.GetThreadVariablesQuery) (model.Page[*model.ThreadVariables], error) {
	sql, args, err := prepareThreadVariablesSearhcQuery(query)
	if err != nil {
		return model.Page[*model.ThreadVariables]{}, errors.Internal("preparing search thread variable query", errors.WithCause(err), errors.WithID("postgres.thread_variables.search"))
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return model.Page[*model.ThreadVariables]{}, errors.Internal(
			"query thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.search"),
		)
	}

	vars, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadVariables])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Page[*model.ThreadVariables]{}, errors.NotFound(
				"zero thread variables found for coresponding filters",
				errors.WithCause(err),
				errors.WithID("postgres.thread_variables.search"),
			)
		}

		return model.Page[*model.ThreadVariables]{}, errors.Internal(
			"collect thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.search"),
		)
	}

	return buildPage(vars, query.Limit), nil
}

func prepareThreadVariablesSearhcQuery(query model.GetThreadVariablesQuery) (string, []any, error) {
	sb := threadVarsEntity.
		SelectFrom("im_thread.thread_variables").
		Select(performColumnsValidation(query.Fields, threadVarsEntity)...)

	if err := applyPagination(sb, query.Pagination, threadVarsEntity); err != nil {
		return "", nil, err
	}

	if len(query.ThreadIDs) > 0 {
		sb.Where(sb.Any("thread_id", "=", query.ThreadIDs))
	}

	if query.DomainID > 0 {
		sb.Where("thread_id in (select id from im_thread.thread where domain_id = " + sb.Var(query.DomainID) + ")")
	}

	if query.CallerID != uuid.Nil {
		c := sb.Var(query.CallerID)
		sb.Where("thread_id in (select acl.thread_id from im_thread.thread_dialog acl where acl.member_id = " + c + " and acl.deleted_at is null union select pv.thread_id from im_thread.thread_preview pv where pv.contact_id = " + c + " and pv.revoked_at is null and pv.expires_at > now())")
	}

	stmt, args := sb.Build()

	return stmt, args, nil
}

func (s *threadVariablesStore) Locate(ctx context.Context, query model.LocateThreadVariablesQuery) (*model.ThreadVariables, error) {
	sql, args := prepareThreadVariablesLocateQuery(query)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Internal(
			"query thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.locate"),
		)
	}

	vars, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadVariables])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}

		return nil, errors.Internal(
			"collect thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.locate"),
		)
	}

	return vars, nil
}

func prepareThreadVariablesLocateQuery(query model.LocateThreadVariablesQuery) (string, []any) {
	sb := threadVarsEntity.
		SelectFrom("im_thread.thread_variables").
		Select(threadVarsEntity.WithoutTag("select-ignore").Columns()...)

	sb.Where(sb.Equal("thread_id", query.ThreadID))

	if query.DomainID > 0 {
		sb.Where("thread_id in (select id from im_thread.thread where domain_id = " + sb.Var(query.DomainID) + ")")
	}

	sb.Limit(1)

	return sb.Build()
}

func (s *threadVariablesStore) Flush(ctx context.Context, flushCmd model.FlushVariablesCommand) (*model.ThreadVariables, error) {
	query, args := prepareThreadVariablesFLushQuery(flushCmd)

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"query flush thread variables",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.flush"),
		)
	}

	vars, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadVariables])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.Forbidden(
				"flushing without being thread member",
				errors.WithCause(err),
				errors.WithID("postgres.thread_variables.flush"),
				errors.WithValue("member_id", flushCmd.Member),
			)
		}

		return nil, errors.Internal(
			"collect flush thread variables result",
			errors.WithCause(err),
			errors.WithID("postgres.thread_variables.flush"),
		)
	}

	return vars, nil
}

func prepareThreadVariablesFLushQuery(flushCmd model.FlushVariablesCommand) (string, pgx.NamedArgs) {
	query := `
		with thread_info as (
			select
				tv.thread_id,
				tv.variables,
				(
					select array_agg(distinct td.member_id)
					from im_thread.thread_dialog td
					where td.thread_id = tv.thread_id
						and td.deleted_at is null
				) as thread_members
			from im_thread.thread_variables tv
			where tv.thread_id = @ThreadID
			for update of tv
		)
		update im_thread.thread_variables tv
		set variables = (
			select coalesce(jsonb_object_agg(t.key, t.val), '{}'::jsonb)
			from jsonb_each(ti.variables) as t(key, val)
			where not (
				(@Keys::text[] is null or t.key = any(@Keys::text[]))
					and (t.val ->> 'set_by')::uuid = @Member
			)
		)
		from thread_info ti
		where tv.thread_id = ti.thread_id
		and @Member = any(ti.thread_members)
		returning tv.thread_id, tv.variables, ti.thread_members;
	`

	args := pgx.NamedArgs{
		"ThreadID": flushCmd.ThreadID,
		"Member":   flushCmd.Member,
		"Keys":     pgtype.FlatArray[string](flushCmd.Keys),
	}

	return query, args
}
