package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

type botControlStore struct {
	db Querier
}

func NewBotControlStore(db Querier) *botControlStore {
	return &botControlStore{db: db}
}

type botControlStackRecord struct {
	ID        uuid.UUID  `db:"id"`
	ThreadID  uuid.UUID  `db:"thread_id"`
	MemberID  *uuid.UUID `db:"member_id"`
	Position  int        `db:"position"`
	ContactID *uuid.UUID `db:"contact_id"`
}

// Push adds a new entry onto the stack and updates thread.bot_controller_id and
// thread.owner_bot_id — all in a single CTE round-trip.
// Returns the previous top entry.
func (s *botControlStore) Push(ctx context.Context, transition model.BotControlTransition) (*model.BotControlPushResult, error) {
	// The owner_bot_id subquery runs against the pre-insert snapshot, so it correctly
	// identifies the earliest bot in thread_dialog not yet on the stack.
	// For the initial push this is the bot being added; for subsequent transfers
	// owner_bot_id is already set and COALESCE is a no-op.
	rows, err := s.db.Query(ctx, `
		WITH prev_top AS (
			SELECT id, thread_id, member_id, position
			FROM im_thread.bot_control_stack
			WHERE thread_id = @ThreadID
			ORDER BY position DESC
			LIMIT 1
		),
		ins AS (
			INSERT INTO im_thread.bot_control_stack (thread_id, member_id, position)
			VALUES (
				@ThreadID,
				@MemberID,
				(SELECT COALESCE(MAX(position) + 1, 0) FROM im_thread.bot_control_stack WHERE thread_id = @ThreadID)
			)
		),
		upd AS (
			UPDATE im_thread.thread
			SET bot_controller_id = @MemberID,
			    owner_bot_id = COALESCE(owner_bot_id, (
			        SELECT d.id
			        FROM im_thread.thread_dialog d
			        WHERE d.thread_id = @ThreadID
			          AND d.is_bot = true
			          AND d.deleted_at IS NULL
			          AND d.id NOT IN (
			              SELECT s.member_id
			              FROM im_thread.bot_control_stack s
			              WHERE s.thread_id = @ThreadID AND s.member_id IS NOT NULL
			          )
			        ORDER BY d.created_at ASC
			        LIMIT 1
			    ))
			WHERE id = @ThreadID
			RETURNING id
		)
		SELECT p.id, p.thread_id, p.member_id, COALESCE(p.position, 0) AS position, d.member_id AS contact_id
		FROM upd u
		LEFT JOIN prev_top p ON true
		LEFT JOIN im_thread.thread_dialog d ON d.id = p.member_id
	`, pgx.NamedArgs{
		"ThreadID": transition.ThreadID,
		"MemberID": transition.NewMemberID,
	})
	if err != nil {
		return nil, errors.Internal("push bot control", errors.WithCause(err), errors.WithID("bot_control_store.push"))
	}

	row, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[botControlStackRecord])
	if err != nil {
		return nil, errors.Internal("scanning push result", errors.WithCause(err), errors.WithID("bot_control_store.push"))
	}

	result := &model.BotControlPushResult{}
	if row.MemberID != nil {
		result.Prev = mapBotControlStackEntry(row)
	}

	return result, nil
}

// Pop removes the stack entry for the given memberID.
// If the member was the current top, updates bot_controller_id to the new top (or owner_bot_id if stack is now empty).
// If the member's dialog is marked auto_leave, it is soft-deleted.
// Returns the new top entry after removal (nil if stack is now empty and no owner bot).
func (s *botControlStore) Pop(ctx context.Context, threadID, memberID uuid.UUID, reason model.BotControlReason, _ *uuid.UUID) (*model.BotControlStackEntry, error) {
	// Step 1: fetch entry, delete it, and conditionally soft-delete its dialog — all in one CTE.
	// soft_del uses a NULL-safe trick: if auto_leave=false the subquery returns NULL,
	// so "WHERE id = NULL" matches nothing (no-op update without branching in Go).
	// new_top must run in a separate statement because CTEs share a single snapshot —
	// a SELECT within the same statement would still see the deleted row.
	type popRecord struct {
		botControlStackRecord

		AutoLeave bool `db:"auto_leave"`
		IsTop     bool `db:"is_top"`
	}

	entryRows, err := s.db.Query(ctx, `
		WITH entry AS (
			SELECT s.id, s.thread_id, s.member_id, s.position,
			       COALESCE(d.auto_leave, false) AS auto_leave,
			       s.position = COALESCE((SELECT MAX(position) FROM im_thread.bot_control_stack WHERE thread_id = @ThreadID), -1) AS is_top
			FROM im_thread.bot_control_stack s
			LEFT JOIN im_thread.thread_dialog d ON d.id = s.member_id
			WHERE s.thread_id = @ThreadID AND s.member_id = @MemberID
			LIMIT 1
		),
		del AS (
			DELETE FROM im_thread.bot_control_stack
			WHERE id = (SELECT id FROM entry)
		),
		soft_del AS (
			UPDATE im_thread.thread_dialog
			SET deleted_at = NOW(), leave_reason = @Reason
			WHERE id = (SELECT member_id FROM entry WHERE auto_leave = true)
			  AND deleted_at IS NULL
		)
		SELECT id, thread_id, member_id, position, auto_leave, is_top FROM entry
	`, pgx.NamedArgs{"ThreadID": threadID, "MemberID": memberID, "Reason": string(reason)})
	if err != nil {
		return nil, errors.Internal("pop bot control", errors.WithCause(err), errors.WithID("bot_control_store.pop"))
	}

	entry, err := pgx.CollectOneRow(entryRows, pgx.RowToAddrOfStructByNameLax[popRecord])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.NotFound("member not found in bot control stack", errors.WithID("bot_control_store.pop"))
		}

		return nil, errors.Internal("scanning stack entry", errors.WithCause(err), errors.WithID("bot_control_store.pop"))
	}

	isTop := entry.IsTop

	// Step 4: find new top, update bot_controller_id, and fetch the dialog
	// context needed for bot.control.granted.v1 — all in one CTE round-trip.
	// new_top JOINs thread_dialog to get contact_id/auto_leave/domain_id.
	// owner_info does the same for the owner bot fallback (stack empty + isTop case).
	// For client_leave we fully release control: when the stack empties, bot_controller_id
	// is set to NULL (no owner-bot fallback) so a later message re-pushes via ensureBotControl.
	type newTopResult struct {
		MemberID   *uuid.UUID `db:"member_id"`
		Position   int        `db:"position"`
		OwnerBotID *uuid.UUID `db:"owner_bot_id"`
		ContactID  *uuid.UUID `db:"contact_id"`
		AutoLeave  bool       `db:"auto_leave"`
		DomainID   int        `db:"domain_id"`
	}

	newTopRows, err := s.db.Query(ctx, `
		WITH new_top AS (
			SELECT s.member_id, s.position,
			       d.member_id AS contact_id, d.auto_leave, d.domain_id
			FROM im_thread.bot_control_stack s
			JOIN im_thread.thread_dialog d ON d.id = s.member_id
			WHERE s.thread_id = @ThreadID
			ORDER BY s.position DESC
			LIMIT 1
		),
		upd AS (
			UPDATE im_thread.thread
			SET bot_controller_id = COALESCE(
				(SELECT member_id FROM new_top),
				CASE
					WHEN @IsClientLeave THEN NULL
					WHEN @IsTop         THEN owner_bot_id
					ELSE bot_controller_id
				END
			)
			WHERE id = @ThreadID
			RETURNING owner_bot_id
		),
		owner_info AS (
			SELECT d.member_id AS contact_id, d.auto_leave, d.domain_id
			FROM im_thread.thread_dialog d
			JOIN upd u ON d.id = u.owner_bot_id
			WHERE u.owner_bot_id IS NOT NULL
		)
		SELECT
			n.member_id,
			COALESCE(n.position, 0)                              AS position,
			u.owner_bot_id,
			COALESCE(n.contact_id,  oi.contact_id)              AS contact_id,
			COALESCE(n.auto_leave,  oi.auto_leave,  false)       AS auto_leave,
			COALESCE(n.domain_id,   oi.domain_id,   0)::int      AS domain_id
		FROM upd u
		LEFT JOIN new_top n ON true
		LEFT JOIN owner_info oi ON true
	`, pgx.NamedArgs{
		"ThreadID":      threadID,
		"IsTop":         isTop,
		"IsClientLeave": reason == model.BotControlReasonClientLeave,
	})
	if err != nil {
		return nil, errors.Internal("fetching new top after pop", errors.WithCause(err), errors.WithID("bot_control_store.pop"))
	}

	result, err := pgx.CollectOneRow(newTopRows, pgx.RowToAddrOfStructByNameLax[newTopResult])
	if err != nil {
		return nil, errors.Internal("scanning new top after pop", errors.WithCause(err), errors.WithID("bot_control_store.pop"))
	}

	contactID := uuid.Nil
	if result.ContactID != nil {
		contactID = *result.ContactID
	}

	if result.MemberID != nil {
		return &model.BotControlStackEntry{
			ThreadID:  threadID,
			MemberID:  result.MemberID,
			Position:  result.Position,
			ContactID: contactID,
			AutoLeave: result.AutoLeave,
			DomainID:  result.DomainID,
		}, nil
	}

	// Stack is empty — synthesize owner bot entry so the service fires a granted event.
	if isTop && result.OwnerBotID != nil && reason != model.BotControlReasonClientLeave {
		return &model.BotControlStackEntry{
			ThreadID:  threadID,
			MemberID:  result.OwnerBotID,
			Position:  0,
			ContactID: contactID,
			AutoLeave: result.AutoLeave,
			DomainID:  result.DomainID,
		}, nil
	}

	return nil, nil //nolint:nilnil
}

// GetStack returns all stack entries for a thread ordered by position ascending.
func (s *botControlStore) GetStack(ctx context.Context, threadID uuid.UUID) ([]*model.BotControlStackEntry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.thread_id, s.member_id, s.position, d.member_id AS contact_id
		FROM im_thread.bot_control_stack s
		LEFT JOIN im_thread.thread_dialog d ON d.id = s.member_id
		WHERE s.thread_id = @ThreadID
		ORDER BY s.position ASC
	`, pgx.NamedArgs{"ThreadID": threadID})
	if err != nil {
		return nil, errors.Internal("querying stack", errors.WithCause(err), errors.WithID("bot_control_store.get_stack"))
	}

	records, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[botControlStackRecord])
	if err != nil {
		return nil, errors.Internal("collecting stack rows", errors.WithCause(err), errors.WithID("bot_control_store.get_stack"))
	}

	entries := make([]*model.BotControlStackEntry, 0, len(records))
	for _, r := range records {
		entries = append(entries, mapBotControlStackEntry(r))
	}

	return entries, nil
}

func (s *botControlStore) ClearController(ctx context.Context, threadID uuid.UUID) (*uuid.UUID, error) {
	type clearedRecord struct {
		MemberID uuid.UUID `db:"member_id"`
	}

	rows, err := s.db.Query(ctx, `
		WITH prev AS (
			SELECT bot_controller_id AS member_id
			FROM im_thread.thread
			WHERE id = @ThreadID AND bot_controller_id IS NOT NULL
		),
		upd AS (
			UPDATE im_thread.thread
			SET bot_controller_id = NULL
			WHERE id = @ThreadID AND bot_controller_id IS NOT NULL
		)
		SELECT member_id FROM prev
	`, pgx.NamedArgs{"ThreadID": threadID})
	if err != nil {
		return nil, errors.Internal("clear bot controller", errors.WithCause(err), errors.WithID("bot_control_store.clear_controller"))
	}

	record, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[clearedRecord])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil // no controller to clear
		}

		return nil, errors.Internal("scanning cleared controller", errors.WithCause(err), errors.WithID("bot_control_store.clear_controller"))
	}

	return &record.MemberID, nil
}

func mapBotControlStackEntry(r *botControlStackRecord) *model.BotControlStackEntry {
	e := &model.BotControlStackEntry{
		ID:       r.ID,
		ThreadID: r.ThreadID,
		MemberID: r.MemberID,
		Position: r.Position,
	}
	if r.ContactID != nil {
		e.ContactID = *r.ContactID
	}

	return e
}
