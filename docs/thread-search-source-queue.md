# Source and Queue in Thread Search

`ThreadManagement.Search` returns two extra objects per thread so the agent's
active-chat list can show (and sort by) where a conversation came from and
through which call_center queue the agent received it:

- `source {id, name, type}` — the provider gate (`id` = gate id from
  `thread_dialog.via`, `name` = its display name, `type` = provider type such as
  `telegram_bot`).
- `queue {id, name}` — the call_center queue through which the **requesting
  agent** (`self_id`) received the thread.

Plan: `docs/dev-task-plans/thread-search-source-queue.md`. Client-facing API
reference: `docs/thread_search.md`.

## Why local reference tables

The thread service does not own gates or queues: their names live in the
providers and call_center services. Search is a hot path and cannot call other
services per row, so the display data is kept in two small local tables
(`source_ref`, `queue_ref`) that are refreshed whenever a write already carries
the data (an inbound message with gate info, or an agent assignment through a
queue). Names are therefore **eventually consistent**: they update on the next
write that carries them.

## What changed

### Protos (local branch only)

The protos changes exist only in the local clone
`/Users/dtsnk0/Documents/projects/protos`, branch
`feat/thread-search-source-queue` (uncommitted, not pushed). Only
`im/service/thread/v1/` is changed:

| File | Change |
|------|--------|
| `peer.proto` | `message Via {string id = 1; string name = 2; string type = 3;}`, `Identity.via_ref = 5`; `Identity.via = 3` marked `deprecated` (kept as a fallback for clients that send only the gate id) |
| `shared.proto` | `message ThreadQueue {int64 id = 1; string name = 2;}` |
| `thread_service.proto` | `message ThreadSource {string id = 1; string name = 2; string type = 3;}`, `Thread.source = 21`, `Thread.queue = 22`, `AddMemberRequest.queue = 8`, `TransferRequest.queue = 7` |
| `thread_preview_service.proto` | `UpgradePreviewToFullMemberRequest.queue = 6` |

`GivePreviewRequest` and `im/api/gateway` are untouched. `gen/go/thread/v1/*.pb.go`
was regenerated from that local branch (`buf/`, `Makefile` unchanged).

### Schema (`migrations/20261015120000_create_source_queue_refs.sql`)

- `im_thread.source_ref` — PK `gate_id`, `domain_id`, `name`, `type`,
  `deleted`, `updated_at`; index `(domain_id, name)`.
- `im_thread.queue_ref` — PK `(domain_id, queue_id)`, `name`, `deleted`,
  `updated_at`; index `(domain_id, name)`.
- `im_thread.thread_dialog.queue_id bigint NULL` — queue through which this
  member received the thread.
- `thread_preview` gets no new column. `Down` reverses all three changes.

### Store

- `store.ReferenceStore` (`internal/store/store.go`) exposed through
  `UnitOfWork.References()`; implemented in
  `internal/store/postgres/reference_store.go`.
- `UpsertSource` / `UpsertQueue` use
  `insert … on conflict … do update … where (…) is distinct from (…)`, so
  `updated_at` changes only when the data actually changes, and a re-seen ref
  clears `deleted`.
- `thread_dialog_store.go` writes `queue_id` on insert.

### Write path (`internal/service`)

- `ReferenceCache` (`reference_cache.go`) — process-local Ristretto cache
  (4096 entries, 30 min TTL) of the last written value per gate / queue.
  Checked before each upsert; equal value → no SQL. All methods are nil-safe;
  a nil cache means "always upsert".
- `refreshSource` / `refreshQueue` (methods of `ThreadManagementService`) are
  the only writers of the directories: check the cache → one standalone upsert
  (autocommit, outside the caller's transaction) → remember the value. A gate
  or queue name is true on its own, so it need not be atomic with the
  thread / member write; a failed caller transaction leaves a correct row in
  the directory. Running outside the long assignment transaction also avoids
  holding the `queue_ref` row lock while it runs. Upsert errors are logged at
  Warn and never fail the caller.
- **Source**, single hook in `EnsureDirectThread` (`MessageService` unchanged):
  - both for a new thread (before the creation transaction) and for an
    existing one, `refreshSource` is called once; the upsert runs only on a
    cache miss.
  - The ref is built by `sourceRefFromPeers` from the first peer that has
    a non-empty `ViaInfo.ID` and `ViaInfo.Name`. No gate name (e.g. outbound or
    agent-originated sends) → no upsert.
  - Source is written only on this direct-thread path. `AddMember` and
    `UpgradePreviewToFullMember` carry no source by design: external-messenger
    users are always the dialog owner and are never added as separate members,
    and the preview upgrade is a system call that works only for operators.
- **Queue**, `AddMember`, `UpgradePreviewToFullMember` (through
  `addMemberToChat`) and `Transfer` accept an optional `Queue *model.QueueInfo`.
  With `ID > 0` the new member row gets `queue_id` inside the member
  transaction, and `refreshQueue` refreshes `queue_ref` (when the name is
  non-empty) right before that transaction. Without a queue `queue_id` stays NULL. `Transfer` takes the queue
  only from its request (it is not inherited from the initiator). `GivePreview`
  is unchanged.

### Read path (`internal/store/query_object/thread.go`)

- New fields `source`, `queue`; new sort-only keys `source_name`,
  `queue_name` (`fieldMetadata.sortOnly`, `fieldMetadata.nullsLast` in
  `query_object.go`). Sort-only keys are ignored in `fields`.
- `source`: lateral picks the first non-bot `thread_dialog` row with a
  non-empty `via` (active rows first, then by `created_at`), then
  `left join source_ref src … and src.deleted = false`. Returned only when a
  ref row exists.
- `queue`: only for self-scoped queries (`selfScoped` is set by
  `WithContactIDFilter`, `WithSharedMembersFilter`, `WithParticipantsFilter`,
  i.e. Search). `left join queue_ref qr on qr.domain_id = t.domain_id and
  qr.queue_id = td.queue_id and qr.deleted = false`, selected as
  `case when qr.queue_id is null then null else jsonb_build_object(...) end`.
  No ref row → `queue` is NULL/nil, never `{id, name: ""}`.
- `Get` with `caller_id` calls `WithQueueOwner(caller)`: `queue` (when
  requested via `fields`) is resolved from the caller's own `thread_dialog`
  row through a `LEFT JOIN … qtd` (`deleted_at is null`). It never filters the
  thread out (a preview owner without a member row gets `queue = null`) and
  never multiplies rows, because `(domain_id, thread_id, member_id)` is unique.
- Queries with no caller (`Get` without `caller_id`, bot control) select
  `null::jsonb as queue` without a join.
- Defaults: `source` and `queue` are appended to `DefaultFields()` only when the
  query is self-scoped (Search). Joins are added only when the fields or sort
  keys are requested or defaulted.
- Sorting by `source_name` / `queue_name` uses `NULLS LAST` in both directions.

### gRPC mappers (`internal/handler/grpc/mapper`)

- `MapPeerFromProto` builds a single `shared.Identity.ViaInfo{ID, Name, Type}` from `Identity.via_ref`. The deprecated `Identity.via` (sent by old clients) goes into `ViaInfo.ID` only when `via_ref.id` is empty, so on conflict `via_ref.id` wins. `ViaInfo` is nil when id, name and type are all empty. The domain `shared.Identity` has no separate `Via` field; `Peer.ResolveVia()` returns `ViaInfo.ID`.
- `ConvertAddMemberRequest`, `ConvertTransferThreadRequest`,
  `ConvertUpgradePreviewRequest` map `queue` (nil when absent).
- `ConvertToThread` maps `Source` / `Queue` (nil when absent).

## Acceptance criteria

| # | Criterion | Met |
|---|-----------|-----|
| 1 | Protos: only `im/service/thread/v1/` changed with the listed messages and field numbers; `GivePreviewRequest` and gateway unchanged; `buf build` passes | Yes. `buf lint` reports only pre-existing errors, none in the new definitions |
| 2 | `gen/go/thread/v1` regenerated from the local protos (not hand-edited), all new getters present; `buf/`, `Makefile` unchanged | Yes |
| 3 | Migration creates `source_ref`, `queue_ref`, `thread_dialog.queue_id`; `Down` reverses; no `thread_preview` column | Yes |
| 4 | `UnitOfWork.References()` with idempotent `UpsertSource` / `UpsertQueue`; postgres + test fake | Yes (`fakeReferenceStore` in `thread_manager_add_member_test.go`) |
| 5 | Source refreshed via `refreshSource` in `EnsureDirectThread` (new and existing threads), outside the creation tx, Warn on failure; `MessageService` unchanged | Yes |
| 6 | AddMember / Upgrade / Transfer with queue write `queue_id` in the member tx and refresh `queue_ref` via `refreshQueue` before it; NULL without queue; GivePreview untouched | Yes |
| 7 | Ristretto cache consulted before upsert, written right after a successful standalone upsert, nil = always upsert | Yes (`TestRefreshSource_*`, `TestRefreshQueue_*`, `TestEnsureDirectThread_SourceRefreshIsIndependentOfThreadCreation`) |
| 8 | Search returns `source` / `queue` only when a non-deleted ref row exists (queue only for the caller's own row); both in Search defaults only; lazy joins | Yes. Fixed after user review: `queue` was previously returned without a `queue_ref` row. Verified at SQL-string level only, no DB integration test |
| 9 | `±source_name` / `±queue_name` sort with `NULLS LAST`; sort-only | Yes |
| 10 | `Get` never multiplies rows because of `queue`; with `caller_id` it returns the caller's queue (on request), without `caller_id` `queue` is NULL | Yes |
| 11 | Mappers map `via_ref` (with legacy `via` fallback), `queue`, `Source` / `Queue`, with unit tests | Yes |
| 12 | `go build`, `go vet`, `go test` pass; tests for query SQL, queue propagation, source write path, cache skip | Yes. Source write-path tests added after user review |
| 13 | `docs/thread_search.md` documents fields, defaults, sort keys, nulls last, eventual consistency | Yes |

## Open issues from review

All low severity, none blocking:

1. **No test for a failing source upsert on an existing thread.**
   `touchSourceForExistingThread` should log Warn and return the thread with
   `err == nil` when `ReferenceStore.UpsertSource` fails; this branch is not
   covered (`thread_manager_ensure_direct_test.go`).
2. **`linkSource` joins `source_ref` by `gate_id` only**, without
   `src.domain_id = t.domain_id`, and `UpsertSource` overwrites `domain_id` on
   conflict. Harmless while gate ids are globally unique UUIDs; adding the
   domain predicate would make cross-tenant leakage impossible.
3. **Peer choice mismatch.** `sourceRefFromPeers` takes the first peer with
   `ViaInfo.ID` + `ViaInfo.Name`, while `initializeDirectThreadDialogs` stores
   `thread_dialog.via` from the first peer with just `ViaInfo.ID`. If `from` has a via
   without gate name and `to` has a different via with gate name, the ref is
   written for a gate Search will not look up. Edge case; both vias are
   normally equal.
4. **No tie-breaker for `source_name` / `queue_name` sorting.** Equal names and
   the many NULLs (threads without refs yet) have no deterministic order, so
   OFFSET pagination can duplicate or skip rows between pages. Existing sorts
   behave the same; consider appending `t.id`.
5. **Unrelated generated changes.** Regenerating from the local protos branch
   (based on a newer protos `main`, `935983c`) pulled in unrelated API changes
   in `gen/go/thread/v1/updates_service.pb.go` (e.g. `DeletedMessage`). Build
   passes, but confirm these are expected or split them out.

### Before merge

- Commit and push the protos branch, open and merge the protos PR.
- Regenerate `gen/go` from protos `main` (`make gen-thread`) and check the diff
  matches the local-branch output.

## How to use

Search request with defaults (source and queue included automatically):

```json
{ "self_id": "<agent contact uuid>", "sort": "-last_msg_at" }
```

Sort by queue name, then fetch explicit fields:

```json
{
  "self_id": "<agent contact uuid>",
  "fields": ["id", "subject", "source", "queue"],
  "sort": "+queue_name"
}
```

Response fragment:

```json
{
  "id": "…",
  "source": { "id": "<gate id>", "name": "Support bot", "type": "telegram_bot" },
  "queue": { "id": 42, "name": "Sales" }
}
```

`source` / `queue` are absent when no reference row exists yet.

Callers that write the data:

- providers/gateway: send `Identity.via_ref {id, name, type}` on inbound
  messages instead of the deprecated `Identity.via`;
- call_center: pass `queue {id, name}` in `AddMember`,
  `UpgradePreviewToFullMember` and `Transfer`.

## How to verify

```sh
go build ./... && go vet ./... && go test ./...
go test ./internal/store/query_object/ -run 'Source|Queue|SortOnly|Defaults|Unscoped'
go test ./internal/service/ -run 'EnsureDirectThread|Queue|ReferenceCache|TouchSourceRef|TouchQueueRef|SourceRefFromPeers|QueueIDOf'
go test ./internal/handler/grpc/mapper/ -run 'ViaRef|LegacyVia|Queue|SourceAndQueue'
```

Manual check against a database with the migration applied:

1. Send an inbound message whose peer carries `via_ref {id, name, type}` → a row
   appears in `im_thread.source_ref`; Search returns `source`.
2. `AddMember` with `queue {id, name}` → `thread_dialog.queue_id` set for the
   new member and a `queue_ref` row exists; Search as that agent returns
   `queue`. Search as another member does not.
3. Delete (or soft-delete) the `queue_ref` row → Search omits `queue` for that
   thread (never `{id, name: ""}`).
4. Repeat the same inbound message → no new SQL (cache hit) and `updated_at`
   in `source_ref` does not change.
