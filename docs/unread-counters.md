# Unread Chats & Messages Counter

Per-participant unread counts: how many unread messages a participant has in
each chat, and how many chats/messages are unread in total. Modelled on the
canonical messenger design — Telegram's `read_inbox_max_id` + dialog
`unread_count`, Slack's channel `last_read` + unread — a **read horizon** plus a
**denormalized counter** on the participant's dialog row.

## The model

Each `im_thread.thread_dialog` row (one per participant per thread) carries:

- `last_read_message_id UUID` — the **read horizon**: the newest message the
  member has read (read-up-to boundary). Advances **monotonically**, never
  backward.
- `unread_count INTEGER` — the **denormalized** number of unread messages,
  maintained in the same transactions that change state (recompute-on-write).

Unread is derived from the horizon, not from a per-message scan:

```
unread = messages in the thread with id > last_read_message_id,
         not sent by the member, that are not system messages
```

Message ids are UUIDv7 (time-ordered), so `id > horizon` is exactly "arrived
after the last thing I read". This is the same shape Telegram and Slack use.

### Two separate concerns

| Concern | Telegram term | Where it lives here |
|---------|---------------|---------------------|
| My unread (inbox) | `read_inbox_max_id` + `unread_count` | `thread_dialog.last_read_message_id` + `unread_count` |
| Who read my messages (ticks) | `read_outbox_max_id` | `im_message.message_statuses` (per-recipient delivery state) |

`message_statuses` stays exactly as delivery status left it — it drives the
sender's read receipts / ticks. Unread does **not** read from it; it reads the
denormalized counter.

## How the counter is maintained

All updates happen **in the same transaction** as the change that triggered
them (`internal/store/postgres/message_status_store.go`):

| Event | Effect on `unread_count` | How |
|-------|--------------------------|-----|
| New content message (`InsertSent`) | `+1` for each recipient | single `UPDATE … SET unread_count = unread_count + 1`; skipped for system messages; the sender is not a recipient |
| Read (`MarkRead`, read-up-to) | advance horizon + **recompute** | `advanceReadHorizon`: move `last_read_message_id` forward monotonically, then recount messages after the new horizon |
| `MarkDelivered` / `MarkFailed` | none | unread depends on the read horizon, not on delivery state |

The read path recomputes from the horizon rather than decrementing, so it is
self-correcting: it can never drift below zero or miss a concurrent insert.

```sql
-- advanceReadHorizon (per receipt, monotonic): move the horizon forward and
-- recount in one UPDATE
update im_thread.thread_dialog td
set last_read_message_id = greatest_horizon(td.last_read_message_id, r.up_to),
    unread_count = (
        select count(*) from im_message.messages m
        where m.thread_id = td.thread_id
          and m.sender_id <> td.member_id
          and m.type <> 4                       -- not system
          and m.id > greatest_horizon(td.last_read_message_id, r.up_to)
    )
from unnest(:threads, :members, :up_tos) as r(thread_id, member_id, up_to)
where td.thread_id = r.thread_id and td.member_id = r.member_id
  and td.deleted_at is null
```

(`greatest_horizon` is written inline as a `CASE` — the UPDATE target isn't in
scope of a lateral FROM item.)

### Read paths (cheap — no message scan)

- `ReadUnread(domainID, memberID, threadIDs)` → `map[thread]count`, read straight
  from `thread_dialog.unread_count`. Used to enrich `Thread.unread_count` in the
  thread list.
- `UnreadSummary(domainID, memberID)` → `{ unread_chats, unread_messages }` =
  `count(*) filter (where unread_count > 0)` and `sum(unread_count)` over the
  member's active dialogs.

### Drift safety net

`ReconcileUnread(domainID)` recomputes `unread_count` for every active dialog
from the horizon. Intended for a periodic job (not wired to a cron yet) so any
drift — from a backfill, a manual fix, or a new code path that touches statuses
without going through these methods — self-heals.

## API surface

Unchanged from the first iteration — the horizon work is internal:

- `Thread.unread_count` — field 16 (service), 14 (gateway); filled for the
  requesting participant (`ThreadSearchRequest.self_id`).
- `GetUnreadSummary` — service `{ self_id, domain_id }` → `{ unread_chats,
  unread_messages }`; gateway empty request, `GET /v1/threads/unread`.

## Real-time

No dedicated unread event. The client keeps the badge live off existing stream
events and treats the server as authoritative on load:

```
new message  → message_event         → client +1
read up to X → message_status_event  → client resets (READ, own member_id)
load/reconnect → Thread.unread_count / GetUnreadSummary  (authoritative)
```

im-delivery is unchanged: it already relays both events and fans read receipts
out to all of the reader's sessions (multi-device).

## Migration

`migrations/20260727120000_add_unread_horizon_to_thread_dialog.sql`:

1. `ALTER TABLE thread_dialog ADD last_read_message_id UUID, unread_count INTEGER NOT NULL DEFAULT 0`.
2. Backfill the horizon from existing read state (`max(message_id)` where
   `status = 3`, per thread/member).
3. Backfill `unread_count` from that horizon.

## Changes by service

**protos/im** — `Thread.unread_count`, `GetUnreadSummary` (unchanged from the
first iteration).

**im-thread-service**
- `migrations/…_add_unread_horizon_to_thread_dialog.sql` — horizon + counter columns + backfill.
- `internal/store/postgres/message_status_store.go` — `InsertSent` bump,
  `advanceReadHorizon` in `MarkRead`, `ReadUnread`, `UnreadSummary`,
  `ReconcileUnread`.
- `internal/store/store.go` — interface (`ReadUnread`, `UnreadSummary`, `ReconcileUnread`).
- `internal/domain/model/message_status.go` — `UnreadSummary` type.
- `internal/domain/model/thread.go` — `Thread.UnreadCount`.
- `internal/service/thread_manager.go` — `enrichUnread` (reads the denormalized
  counter), `GetUnreadSummary`.
- handler + mapper + dto — surface the field and RPC.

**im-gateway-service** — pass-through of `unread_count` and `GetUnreadSummary`
(unchanged from the first iteration).

## Not in this change / follow-ups

- **Mentions counter** (`unread_mentions_count`, Telegram-style) — the schema
  leaves room, but there are no @-mentions in the system yet.
- **ReconcileUnread cron** — the method exists; wiring it to a schedule is a
  small follow-up.
- **No im-delivery change** — real-time reuses existing events.

## Testing

- Service-level unit tests (`internal/service/thread_manager_unread_test.go`):
  `enrichUnread` sets counts from `ReadUnread`, skips the store with no
  `self_id`, swallows errors; `GetUnreadSummary` requires `self_id`.
- `go build`, `go vet`, `go test ./...` pass in im-thread-service and im-gateway-service.
- The SQL (horizon advance, recompute, backfill) has no DB-backed test — the
  repo has no integration harness, consistent with the rest of the SQL layer.
  `ReconcileUnread` is the operational check that the denormalized counter
  matches the source.
