# Source and queue in the active thread list (with protos)

Add `source` (provider gate) and `queue` (call_center queue) to `ThreadManagement.Search`: local reference tables (`source_ref`, `queue_ref`) refreshed on write, `thread_dialog.queue_id` written on AddMember / UpgradePreviewToFullMember / Transfer, lazy JOINs and `source_name`/`queue_name` sorting (nulls last), a Ristretto cache that skips unchanged upserts, thread-service proto changes on the local protos branch `feat/thread-search-source-queue`, `gen/go` regenerated from that local branch, gRPC mappers and docs. GivePreview and gateway protos are not touched.

- **Branch:** feat/thread-search-source-queue
- **Plan path:** docs/dev-task-plans/thread-search-source-queue.md

## Acceptance criteria

- [ ] In the local protos clone `/Users/dtsnk0/Documents/projects/protos` (branch `feat/thread-search-source-queue`, uncommitted, not pushed), only files under `im/service/thread/v1/` are changed: `peer.proto` gains `message ViaInfo {string name = 1; string type = 2;}` and `Identity.via_info = 5`; `shared.proto` gains `message ThreadQueue {int64 id = 1; string name = 2;}`; `thread_service.proto` gains `message ThreadSource {string id = 1; string name = 2; string type = 3;}`, `Thread.source = 21`, `Thread.queue = 22`, `AddMemberRequest.queue = 8`, `TransferRequest.queue = 7`; `thread_preview_service.proto` gains `UpgradePreviewToFullMemberRequest.queue = 6`. `GivePreviewRequest` and everything under `im/api/gateway` are unchanged. `buf lint` and `buf build` in `protos/im` pass.
- [ ] `gen/go/thread/v1/*.pb.go` in im-thread-service is regenerated from the local protos directory (not hand-edited) and contains `ViaInfo`, `ThreadSource`, `ThreadQueue`, `Identity.GetViaInfo()`, `Thread.GetSource()/GetQueue()`, `AddMemberRequest.GetQueue()`, `TransferRequest.GetQueue()`, `UpgradePreviewToFullMemberRequest.GetQueue()`. `buf/buf.gen.thread.yaml`, `buf/thread_gen.go` and `Makefile` are unchanged (`git diff --stat buf Makefile` is empty).
- [ ] A new goose migration creates `im_thread.source_ref` (PK `gate_id`, `domain_id`, `name`, `type`, `deleted`, `updated_at`, index `(domain_id, name)`) and `im_thread.queue_ref` (PK `(domain_id, queue_id)`, `name`, `deleted`, `updated_at`, index `(domain_id, name)`), and adds a nullable `queue_id bigint` to `im_thread.thread_dialog`. `Down` reverses all three. No column is added to `thread_preview`.
- [ ] `store.UnitOfWork` exposes `References() store.ReferenceStore` with `UpsertSource` / `UpsertQueue`, both `insert … on conflict … do update … where (…) is distinct from (…)` so `updated_at` changes only when data changes. Implemented in `internal/store/postgres` and in the service test fake.
- [ ] Direct thread creation (`initializeDirectThreadDialogs`) upserts `source_ref` in the creation transaction when the via-carrying peer has `via` and a non-empty `ViaInfo.Name`. For an existing thread, `EnsureDirectThread` upserts in a separate small transaction only on a cache miss; failures there are logged (Warn) and do not fail the send. `MessageService` is not changed.
- [ ] `AddMember`, `UpgradePreviewToFullMember` and `Transfer` with a queue (`ID > 0`) write `thread_dialog.queue_id` on the new member row and upsert `queue_ref` (when the name is non-empty) in the same transaction as the `thread_dialog` insert. Without a queue, `queue_id` is NULL and behaviour is otherwise unchanged. `GivePreview` code and DTO are untouched.
- [ ] A process-local Ristretto `ReferenceCache` is consulted before each upsert. Equal cached values → no SQL. The cache is written only after `WithinTransaction` returns nil. A nil cache means "always upsert".
- [ ] `ThreadManagement.Search` returns `source {id, name, type}` only when a non-deleted `source_ref` row exists for the via of a non-bot member, and `queue {id, name}` only when the caller's own (`self_id`) `thread_dialog.queue_id` is set AND a non-deleted `queue_ref` row exists for it (same rule as source: no ref → no queue, never `{id, name: ""}`). `source` and `queue` are included in the Search default field set (Search only, per Q8). Joins are added only when the fields/sorts are requested or defaulted.
- [ ] `sort: "+source_name" | "-source_name" | "+queue_name" | "-queue_name"` orders by the reference name with `NULLS LAST` in both directions; these keys are sort-only (ignored in `fields`).
- [ ] `ThreadManagement.Get` never multiplies rows because of `queue`; there `queue` is NULL.
- [ ] gRPC mappers: `MapPeerFromProto` maps `via_info`; `ConvertAddMemberRequest`, `ConvertTransferThreadRequest`, `ConvertUpgradePreviewRequest` map `queue` (nil when absent); `ConvertToThread` maps `Source`/`Queue` (nil when absent). Mapper unit tests cover each.
- [ ] `go build ./...`, `go vet ./...` and `go test ./...` pass; new unit tests cover: query-object SQL for source/queue fields, defaults and nulls-last sorting; service queue_id propagation for AddMember/Upgrade/Transfer; source_ref write path (new thread via `initializeDirectThreadDialogs`, existing thread via `EnsureDirectThread`, no upsert without `via_info` e.g. outbound/agent-originated sends); cache-skip behaviour.
- [ ] `docs/thread_search.md` documents `source`/`queue`, their default status, the `source_name`/`queue_name` sort keys, nulls-last ordering and eventual consistency of names.

## Codebase notes

- **Module:** `github.com/webitel/im-thread-service`, Go 1.26. No CLAUDE.md, no `docs/knowledge/`, no graphify graph in this repo. Nothing from the previous planning round has been implemented yet: `git diff main...HEAD` only shows the unrelated health-probe merge; there is no `source_ref`, `ReferenceCache`, `QueueInfo` or `ViaInfo` anywhere in the code. All line numbers below were re-verified against HEAD `bfa4350`.
- **Build/test/lint:** `go build ./...`, `go vet ./...`, `go test ./...`. Integration tests carry `//go:build integration` (`scripts/test-integration.sh`); do not add DB integration tests. Lint: `golangci-lint run ./...` with `.golangci.yaml` (gofmt, gofumpt, gci import groups: std / third-party / `github.com/webitel/webitel-go-kit/...` / local module). Mirror any existing import block.
- **Proto generation (verified in this planning round):** `buf` 1.59.0 is at `/opt/homebrew/bin/buf`. The committed template `buf/buf.gen.thread.yaml` has `inputs: git_repo https://github.com/webitel/protos.git, branch main, subdir im` and `out: ../gen/go` (relative to the cwd), with a `go_package` override to `github.com/webitel/protos/gen/go/im/thread/v1;thread` and `module=github.com/webitel/protos/gen/go/im`, so output lands in `gen/go/thread/v1` (Go package `thread`, imported as `impb "github.com/webitel/im-thread-service/gen/go/thread/v1"`). **A positional input on the command line overrides the template's `inputs`**, so local generation needs no config change. Run it from the repo's `buf/` dir:
  ```sh
  cd <repo>/buf && buf generate /Users/dtsnk0/Documents/projects/protos/im \
    --template buf.gen.thread.yaml \
    --path /Users/dtsnk0/Documents/projects/protos/im/service/thread/v1
  ```
  `--path` must be absolute (a relative `service/thread/v1` is resolved against the cwd and fails with "outside the context directory"). A dry run of exactly this against the unmodified protos branch produced byte-identical files except two upstream changes already on protos `main` but not yet in committed `gen/go`: `ThreadSearchRequest.unread` (field 14, `thread_service.pb.go`) and `DeletedMessage` / deleted messages in `updates_service.pb.go`. Both are additive and compile with the current code; they will appear in the diff and are expected. Before merge, `gen/go` must be regenerated from protos `main` once the protos PR is merged (the committed template already points there). Do not edit the Makefile `gen-thread` target or `buf/thread_gen.go`.
- **Proto package:** every file in `im/service/thread/v1` is `package webitel.im.service.thread.v1` and generates into one Go package, so new message names must be unique across `peer.proto`, `shared.proto`, `thread_service.proto`, `thread_preview_service.proto` (checked: no `ViaInfo`, `ThreadSource`, `ThreadQueue`, `Queue` or `Source` message exists). `thread_preview_service.proto` imports only `buf/validate/validate.proto` and `service/thread/v1/shared.proto`; `thread_service.proto` imports `peer.proto` and `shared.proto` (among others).
- **Layering:** gRPC handler (`internal/handler/grpc`, mappers in `internal/handler/grpc/mapper`) → service (`internal/service`, DTOs in `internal/service/dto`) → store interfaces (`internal/store/store.go`, `internal/store/uow.go`) → postgres (`internal/store/postgres`). Thread search SQL builder: `internal/store/query_object` (squirrel, `$n`). Domain: `internal/domain/model`, `internal/domain/shared`. `dto` already imports `model`.
- **DI:** uber fx. Services: `internal/service/module.go` `fx.Provide(...)` (list starts at line 17; `NewThreadService`, `NewThreadPreviewService` are plain entries). Adding a constructor parameter whose type another provider returns is enough.
- **Errors:** `github.com/webitel/webitel-go-kit/pkg/errors`: `errors.Internal(msg, errors.WithCause(err), errors.WithID("postgres.<store>.<op>"))`, `errors.InvalidArgument(...)`. IDs follow `layer.component.operation`.
- **Logging:** `*slog.Logger`; services use `t.log().WarnContext(ctx, "...", "thread_id", id, "err", err)`.
- **Transactions:** `uow.WithinTransaction(ctx, func(ctx, uow store.UnitOfWork) error)` (`internal/store/postgres/uow.go:191`). The tx UoW is a fresh `&unitOfWork{…, querier: tx}`, so stores must be lazily created getters (like `ThreadPreviews()` at `uow.go:168-174`), never fields copied from the parent. A failed SQL statement aborts the PG tx, so an upsert error inside a tx must be returned, not logged-and-ignored.
- **Test doubles:** `fakeUnitOfWork` in `internal/service/thread_manager_add_member_test.go:21-157` is the only non-postgres `store.UnitOfWork` implementation in the repo and is shared by the whole `service` package; any new `UnitOfWork` method must be added there or the package will not compile. `fakeThreadDialogStore` (same file, line 259) records `lastCreate`. Services are often built as struct literals (`&ThreadManagementService{uow: ...}`), so new fields must be nil-safe.
- **Migrations:** goose SQL in `migrations/`, `YYYYMMDDHHMMSS_description.sql`, embedded by `migrations/migrations.go`. Style: `-- +goose Up` / `-- +goose StatementBegin` … `-- +goose StatementEnd` / `-- +goose Down`, `if not exists` / `if exists` (see `20261001120000_create_thread_preview_table.sql`). Latest is `20261011155427_index_message_attachments_file_id.sql`.
- **Gate id:** `via` is a text column on `im_thread.thread_dialog`. `shared.Identity.Via *string` (`internal/domain/shared/base.go:22-27`) is filled from proto `Identity.via` in `mapper.MapPeerFromProto` (`internal/handler/grpc/mapper/text_message.go:56-89`, the only place a `shared.Identity` is built from proto). `initializeDirectThreadDialogs` writes via only on non-bot rows (bot rows get nil).
- **Cache library:** `github.com/webitel/webitel-go-kit/pkg/cache` (in go.mod). Interface: `Get(ctx, k) (V, bool, error)`, `Set(ctx, k, v) error`, `SetTTL`, `Delete`, `Close`. Build: `cache.New[string, V]().L1(cache.RistrettoConfig{MaxCost: N, NumCounters: N*10, TTL: ttl}).Build()` (see `infra/webitel/im-contact/client.go:79-111`). Ristretto sets are async: in tests use an injected fake implementing the interface, never assert `Get` right after `Set` on a real Ristretto.
- **pgx scanning:** `pgx.RowToAddrOfStructByNameLax` tolerates struct fields missing from the row but fails on selected columns without a matching struct field. Every column a query object selects must have a field on the scanned struct.

## Decisions

### Q1: The required proto fields are not in `github.com/webitel/protos` yet. How should the proto/gRPC layer be handled?

- **Context:** `gen/go/thread/v1` is generated from `webitel/protos` `main` (`buf/buf.gen.thread.yaml`, `make gen-thread`). `impb.Identity` (`gen/go/thread/v1/peer.pb.go:24-36`) has only `Issuer/Name/Via/ChatName`. `impb.Thread` (`thread_service.pb.go:270+`) has no `Source`/`Queue`. `AddMemberRequest`, `GivePreviewRequest` and `UpgradePreviewToFullMemberRequest` have no `queue`. Changing protos is out of scope. Subtasks A and B do not need protos. Subtask C (all mapper changes) does.
- **Options:** (a) Implement A+B now, and keep C on hold until protos are merged to `main`, then run `make gen-thread` and do C. (b) Generate `gen/go` temporarily from a local or feature branch of protos with the expected fields, and re-generate from `main` before merge (risk that names diverge). (c) Ship only the domain/store/query part (A+B) in this PR and leave the gRPC wiring (C) to a follow-up PR.
- **Answer:** Create a new branch `feat/thread-search-source-queue` in the protos repo (local clone `/Users/dtsnk0/Documents/projects/protos`, already created from up-to-date `main`, NOT pushed) and make all required proto changes there: `Thread.source{id,name,type}` and `Thread.queue{id,name}`, `Identity.via_info{name,type}`, optional `queue{id,name}` in AddMember / UpgradePreviewToFullMember / Transfer requests (thread-service protos). Then regenerate `gen/go` in im-thread-service from that local branch (not from protos main) and implement the gRPC mappers on top. Before merge, gen/go must be regenerated from protos main once the protos PR is merged.

### Q2: Where does the Ristretto "last value" cache live, and when is it updated relative to the transaction?

- **Context:** The upserts run inside `uow.WithinTransaction`. If the cache is set as soon as the upsert statement succeeds and the transaction later rolls back (for example, `addMemberToChat` fails on `sendAddMemberSystemMessage` or `BotControl().Push`), the cache claims a value the DB does not have. Later identical values are then skipped until the TTL expires. Avoiding this needs a "remember after commit" mechanism. `WithinTransaction` has no commit hooks today, and the tx UoW is rebuilt per transaction (`postgres/uow.go` `WithinTransaction`), so a store-level decorator cannot see the commit.
- **Options:** (a) A service-level `ReferenceCache`: check before the upsert inside the tx, and `Remember` only after `WithinTransaction` returns nil (callers collect pending remember funcs). (b) Set the cache immediately inside the tx and accept stale skips after a rare rollback, bounded by TTL. (c) A caching decorator at the store layer (inside `postgres` UoW) with set-on-success semantics, the same staleness as (b).
- **Answer:** Service-level ReferenceCache: check before the upsert; `Remember` only after `WithinTransaction` returns nil (collect remember funcs).

### Q3: Which `thread_dialog.via` defines the thread's source, and what does Search return when `source_ref` has no row yet?

- **Context:** The spec says "source = via of the external (non-bot) member". `initializeDirectThreadDialogs` (`internal/service/thread_manager.go:1820-1858`) now writes via only on non-bot rows. Both rows get it when neither peer is a bot. `ExtractExternalPeers` explicitly handles legacy threads where via sits only on the bot row. Existing threads have `via` but no `source_ref` row until the next inbound message carrying via-info.
- **Options:** (a) Use only non-bot rows with non-empty via, and return `{id: via, name: null, type: null}` when the ref is missing. (b) Prefer non-bot rows and fall back to any row (including the bot) with non-empty via, returning id even without a ref. (c) Non-bot rows only, and return `source` only when a `source_ref` row exists (no id-only objects).
- **Answer:** Non-bot rows only; `source` is returned only when a `source_ref` row exists (no ref → no source).

### Q4: What `queue_id` does the new member row get on `Transfer`?

- **Context:** `Transfer` (`internal/service/thread_manager.go:487-682`) creates a new `thread_dialog` row for the target agent and soft-deletes the initiator. Its proto/DTO (`dto.TransferThreadRequest`) has no queue, and the spec only lists AddMember/GivePreview/Upgrade. The spec says that after a transfer the new agent has "a new queue", which may mean call_center calls AddMember. Leaving Transfer alone makes a transferred thread show an empty queue for the new agent.
- **Options:** (a) Leave NULL (no change to Transfer). (b) Inherit `initiator.QueueID` (requires reading `queue_id` in `GetFullView`). (c) Out of scope for now. Revisit when protos add queue to Transfer.
- **Answer:** Add a new optional `queue{id,name}` parameter to Transfer (proto + dto), describing the queue through which the chat arrives to the target agent. If the field is empty, the new member row gets `queue_id = NULL`.

### Q5: What is done with `queue` on `GivePreview`?

- **Context:** `GivePreview` (`internal/service/thread_preview.go:70-123`) creates no `thread_dialog` row, only `im_thread.thread_preview`. Search lists only threads where `td.member_id = self_id`, so previews never appear in Search. If `UpgradePreviewToFullMember` arrives without a queue, the queue passed at GivePreview is lost unless it is persisted.
- **Options:** (a) Only upsert `queue_ref` (refresh the name). Upgrade must carry its own queue. (b) Also add `queue_id` to `thread_preview` (an extra column in the migration), and let Upgrade fall back to the preview's `queue_id` when its own queue is absent.
- **Answer:** Queue is not needed for preview: GivePreview is NOT changed at all (no queue field, no queue_ref upsert). Queue is stored only when the participant becomes a real member — on UpgradePreviewToFullMember (and AddMember / Transfer).

### Q6: Where on the inbound-message path is the `source_ref` upsert hooked?

- **Context:** Every inbound send (`SendText`, `SendDocument`, and `SendLocation`/`SendContact`/`SendInteractive` via `prepareMessageForSending`, all in `internal/service/message.go`) first calls `ThreadManagementService.EnsureDirectThread` (`thread_manager.go:1549`). Then each opens its own message-save transaction. For an existing thread, no `thread_dialog` row is written on the send path, so "the same UoW as the thread_dialog write" only applies to thread creation.
- **Options:** (a) A single hook in `EnsureDirectThread`: a new thread upserts inside the creation tx (`initializeDirectThreadDialogs`), and an existing thread gets a small separate tx only on a cache miss. `MessageService` stays untouched. (b) Hook into each `Send*` message-save transaction in `message.go` (5 call sites, and `MessageService` gets the cache dependency).
- **Answer:** A single hook in `EnsureDirectThread`: new thread → upsert inside the creation tx (`initializeDirectThreadDialogs`); existing thread → a separate small tx only on a cache miss; MessageService untouched.

### Q7: Should `source` and `queue` be part of Search's default field set?

- **Context:** `threadQueryObject.DefaultFields()` (`internal/store/query_object/thread.go:54-59`) is used when `fields` is empty. Adding the fields there makes every default Search pay for the extra lateral and joins. Leaving them out means clients (gateway `convertToThread`) must request them explicitly. `docs/thread_search.md` lists the defaults.
- **Options:** (a) Opt-in only (not in defaults). (b) Add both to defaults.
- **Answer:** Add both `source` and `queue` to Search's default field set.

### Q8: Should the new default fields also apply to `ThreadManagement.Get` (and other internal users of the thread query object), or only to `Search`?

- **Context:** `threadQueryObject.DefaultFields()` (`internal/store/query_object/thread.go:54-59`) is the single default list for every `NewThreadQueryObject()` user: `Search` (`thread_manager.go:133`), `Get` (`thread_manager.go:104`, empty `req.Fields` → defaults) and the internal `uow.ThreadStore().Get(...)` in `CompleteBotControl` (`thread_manager.go:1289`, always defaults). Appending `"source","queue"` unconditionally (the literal reading of Q7) also makes `Get` and the bot-control path run the source lateral + `source_ref` join, and makes `Get` return `queue: null` (Get is not scoped to a caller row, so a queue cannot be resolved there without multiplying rows). Alternatively `DefaultFields()` can add the two fields only when the query is self-scoped (the same three filters that already set `mustIncludeComputedSubject`), which in practice means only Search. `ToSQL` (`query_object.go:127-129`) calls `DefaultFields()` after all filters have been applied, so the scoped check is reliable.
- **Options:** (a) Unconditional: append to `DefaultFields()` for all callers (Get also returns `source`; `queue` is always null in Get). (b) Search-only: `DefaultFields()` appends `source`/`queue` only when the query object is self-scoped; Get/internal callers keep today's defaults (and can still request `source` explicitly via `fields`).
- **Answer:** (b) Search-only: `DefaultFields()` appends `source`/`queue` only when the query is self-scoped; Get and internal callers keep today's defaults and can still request `source` via fields.

## Subtasks

### Subtask A: Schema, reference stores, cache and service write path

#### Instructions

1. **Migration.** Create `migrations/20261015120000_create_source_queue_refs.sql` in the style of `20261001120000_create_thread_preview_table.sql`:
   - Up (inside one `-- +goose StatementBegin/End`):
     - `create table if not exists im_thread.source_ref (gate_id text primary key, domain_id bigint not null, name text not null, type text not null, deleted boolean not null default false, updated_at timestamptz not null default now());`
     - `create index if not exists idx_source_ref_domain_name on im_thread.source_ref (domain_id, name);`
     - `create table if not exists im_thread.queue_ref (domain_id bigint not null, queue_id bigint not null, name text not null, deleted boolean not null default false, updated_at timestamptz not null default now(), primary key (domain_id, queue_id));`
     - `create index if not exists idx_queue_ref_domain_name on im_thread.queue_ref (domain_id, name);`
     - `alter table im_thread.thread_dialog add column if not exists queue_id bigint;`
   - Down: `alter table im_thread.thread_dialog drop column if exists queue_id; drop table if exists im_thread.queue_ref; drop table if exists im_thread.source_ref;`. Do NOT touch `thread_preview` (Q5).
2. **Domain types.** Create `internal/domain/model/thread_ref.go` (package `model`) with exactly these names (subtask C codes against them):
   ```go
   // SourceRef is one row of im_thread.source_ref: display data of a provider gate.
   type SourceRef struct { GateID string; DomainID int; Name string; Type string }
   // QueueRef is one row of im_thread.queue_ref.
   type QueueRef struct { DomainID int; QueueID int64; Name string }
   // QueueInfo is the call_center queue through which an agent receives a thread.
   type QueueInfo struct { ID int64; Name string }
   ```
3. In `internal/domain/model/thread_dialog.go`, add `QueueID *int64 \`json:"queue_id,omitempty" db:"queue_id"\`` to `ThreadDialogExtended` (struct at lines 38-54, right after `Via`). Not on `ThreadDialog`.
4. In `internal/domain/shared/base.go`:
   - Add `type ViaInfo struct { Name string; Type string }` above `Identity`.
   - Add `ViaInfo *ViaInfo` to `Identity` (lines 22-27, after `Via`).
   - Add `func (p Peer) ResolveViaInfo() *ViaInfo` right after `ResolveVia` (lines 51-58), mirroring it (nil `Identity` → nil).
5. **Store interface.** In `internal/store/store.go`, append:
   ```go
   // ReferenceStore keeps the local display directories (source_ref, queue_ref)
   // refreshed on write. Upserts are idempotent and touch updated_at only on change.
   type ReferenceStore interface {
       UpsertSource(ctx context.Context, ref *model.SourceRef) error
       UpsertQueue(ctx context.Context, ref *model.QueueRef) error
   }
   ```
   In `internal/store/uow.go`, add `References() ReferenceStore` to the `UnitOfWork` interface.
6. **Postgres store.** Create `internal/store/postgres/reference_store.go`, mirroring `thread_tag_store.go` (`type referenceStore struct{ db Querier }`, `var _ store.ReferenceStore = (*referenceStore)(nil)`, `func NewReferenceStore(db Querier) *referenceStore`).
   - `UpsertSource`: if `ref == nil || ref.GateID == ""` → `errors.InvalidArgument("gate id required", errors.WithID("postgres.reference_store.upsert_source"))`. Then `s.db.Exec(ctx, sql, pgx.NamedArgs{"GateID":…, "DomainID":…, "Name":…, "Type":…})`:
     ```sql
     insert into im_thread.source_ref (gate_id, domain_id, name, type)
     values (@GateID, @DomainID, @Name, @Type)
     on conflict (gate_id) do update
       set domain_id = excluded.domain_id, name = excluded.name, type = excluded.type,
           deleted = false, updated_at = now()
     where (source_ref.domain_id, source_ref.name, source_ref.type, source_ref.deleted)
           is distinct from (excluded.domain_id, excluded.name, excluded.type, false)
     ```
   - `UpsertQueue`: validate `ref != nil && ref.QueueID > 0`; same pattern on `(domain_id, queue_id)` setting `name`, `deleted = false`, `updated_at = now()` with `where (queue_ref.name, queue_ref.deleted) is distinct from (excluded.name, false)`.
   - Errors: `errors.Internal("upserting source ref", errors.WithCause(err), errors.WithID("postgres.reference_store.upsert_source"))` and the queue equivalent.
7. In `internal/store/postgres/uow.go`: add `referenceStore store.ReferenceStore` to the `unitOfWork` struct (fields at lines 14-35) and `func (u *unitOfWork) References() store.ReferenceStore` with lazy init exactly like `ThreadPreviews()` (lines 168-174). `WithinTransaction` needs no change.
8. **thread_dialog insert.** In `internal/store/postgres/thread_dialog_store.go` `Create` (lines 48-136):
   - Add `queue_id` to `INSERT INTO im_thread.thread_dialog(domain_id, member_id, thread_id, thread_role, invited_by, via, is_bot, auto_leave` and `@QueueID` to the matching `SELECT … FROM im_thread.thread WHERE id = @ThreadID`.
   - Add `dial.via, dial.queue_id` to the final SELECT list (currently `dial.is_bot, dial.auto_leave,` line).
   - Add `"QueueID": member.QueueID` to the NamedArgs.
   - Add `QueueID *int64 \`db:"queue_id"\`` to the `threadDialog` struct (lines 26-46) and `QueueID: dialog.QueueID` in `mapToThreadDialogExtendedModel` (starts line 139). Other queries scanning into `threadDialog` keep working (Lax scanning).
9. **Cache.** Create `internal/service/reference_cache.go`:
   - Constants `referenceCacheSize = 4096`, `referenceCacheTTL = 30 * time.Minute`.
   - `type sourceRefValue struct{ DomainID int; Name, Type string }`; `type ReferenceCache struct { sources cache.Cache[string, sourceRefValue]; queues cache.Cache[string, string] }`.
   - `func NewReferenceCache() (*ReferenceCache, error)`, built like `isBotCache` in `infra/webitel/im-contact/client.go:79-88`.
   - Nil-receiver-safe methods: `SourceChanged(ctx, ref *model.SourceRef) bool` (nil cache / miss / error / differing value → true), `RememberSource(ctx, ref)`, `QueueChanged(ctx, ref *model.QueueRef) bool` keyed by `fmt.Sprintf("%d:%d", ref.DomainID, ref.QueueID)`, `RememberQueue(ctx, ref)`. Errors from `Get`/`Set` are ignored (treated as miss / best effort).
   - Package helpers in the same file:
     - `func sourceRefFromPeers(domainID int, peers ...*shared.Peer) *model.SourceRef`: first non-nil peer whose `ResolveVia()` is non-nil and non-empty AND `ResolveViaInfo()` is non-nil with non-empty `Name` → `&model.SourceRef{GateID: *via, DomainID: domainID, Name: vi.Name, Type: vi.Type}`; else nil.
     - `func touchSourceRef(ctx context.Context, uow store.UnitOfWork, refs *ReferenceCache, ref *model.SourceRef) (func(), error)`: `ref == nil` or `!refs.SourceChanged(ctx, ref)` → `func(){}`, nil. Otherwise `uow.References().UpsertSource(ctx, ref)`; on error return it; else return `func(){ refs.RememberSource(ctx, ref) }`.
     - `func touchQueueRef(ctx context.Context, uow store.UnitOfWork, refs *ReferenceCache, domainID int, q *model.QueueInfo) (func(), error)`: no-op when `q == nil || q.ID <= 0 || q.Name == ""`; otherwise upsert `&model.QueueRef{DomainID: domainID, QueueID: q.ID, Name: q.Name}` + remember func.
     - `func queueIDOf(q *model.QueueInfo) *int64`: nil when `q == nil || q.ID <= 0`, else `&id`.
   - Q2: callers invoke the returned func only after `WithinTransaction` returned nil.
   - Register `NewReferenceCache` in `internal/service/module.go` `fx.Provide(...)` (e.g. next to `NewThreadService`).
10. **ThreadManagementService** (`internal/service/thread_manager.go`):
    - Struct (lines 36-43): add `refs *ReferenceCache`. `NewThreadService` (line 64): add a trailing `refs *ReferenceCache` parameter and set the field. Update the two test callers `internal/service/thread_manager_unread_test.go:55` and `internal/service/thread_manager_tags_test.go:35` (`NewThreadService(nil, fakeUnitOfWork{...}, nil, nil, nil)` → add a trailing `nil`). `internal/handler/grpc/thread.go:52` is a different `NewThreadService` (handler) — do not touch.
    - **New thread** (`orchestrateDirectThreadCreation`, lines 1666-1730, and `initializeDirectThreadDialogs`, lines 1788-1896): change `initializeDirectThreadDialogs` to return `([]*model.ThreadDialogExtended, func(), error)`. After both `ThreadDialogStore().Create` calls succeed (before the `if toIsBot` bot-control block), call `remember, err := touchSourceRef(ctx, uow, t.refs, sourceRefFromPeers(domainID, from, to))`; return the error if any. Return `remember` as the second value (use `func(){}` on early-return paths with errors — callers ignore it then). The only caller is line 1688: capture it into a variable declared outside the tx closure (`var rememberSource func()`), and after `t.uow.WithinTransaction` returns nil, call it if non-nil. `from` first, then `to` matches the via resolution order at lines 1826-1829 (`from.ResolveVia()` then `to.ResolveVia()`).
    - **Existing thread** (`EnsureDirectThread`, lines 1549-1582): in the `if thread != nil {` branch, before `return thread, nil`, call a new method `t.touchSourceForExistingThread(ctx, req)`:
      ```go
      func (t *ThreadManagementService) touchSourceForExistingThread(ctx context.Context, req *dto.EnsureDirectThreadRequest) {
          ref := sourceRefFromPeers(req.DomainID, req.From, req.To)
          if ref == nil || !t.refs.SourceChanged(ctx, ref) { return }
          var remember func()
          err := t.uow.WithinTransaction(ctx, func(ctx context.Context, uow store.UnitOfWork) error {
              var err error
              remember, err = touchSourceRef(ctx, uow, t.refs, ref)
              return err
          })
          if err != nil { t.log().WarnContext(ctx, "failed to refresh source ref", "gate_id", ref.GateID, "err", err); return }
          remember()
      }
      ```
      Check `dto.EnsureDirectThreadRequest` field names (`From`, `To` are `*shared.Peer`, `DomainID int` — used at lines 1556 and 1606 already).
    - **addMemberToChat** (lines 329-485): declare `var rememberQueue func()` next to `newMember`/`systemMessages` (line ~377). Inside the tx, after the `beforeCreate` block and before `ThreadDialogStore().Create`, call `rememberQueue, err = touchQueueRef(ctx, uow, t.refs, domainID, req.Queue)` and return err. Add `QueueID: queueIDOf(req.Queue),` to the `&model.ThreadDialogExtended{...}` literal. After the tx succeeds (before `t.dispatchSystemMessages`), call `rememberQueue()` if non-nil. `domainID` is the resolved one (`req.DomainID`, overridden by `initiator.DomainID`, lines 343-349).
    - **Transfer** (lines 487-682): same pattern. Inside the tx after `LockForUpdate`, call `touchQueueRef(ctx, uow, t.refs, initiator.DomainID, req.Queue)`; add `QueueID: queueIDOf(req.Queue),` to the `ThreadDialogStore().Create` literal (line ~529); run the remember func after `WithinTransaction` returns nil. Do not read/inherit the initiator's queue (Q4: empty → NULL).
11. **DTOs.**
    - `internal/service/dto/thread.go`: add `Queue *model.QueueInfo` (with a short comment) to `AddMemberRequest` (lines 83-94) and to `TransferThreadRequest` (lines 131-138).
    - `internal/service/dto/thread_preview.go`: add `Queue *model.QueueInfo` to `UpgradePreviewRequest` (lines 26-32) only. Do NOT change `GivePreviewRequest` (Q5).
12. **Previews** (`internal/service/thread_preview.go`): in `UpgradePreviewToFullMember` (lines 174-214) add `Queue: req.Queue,` to the `&dto.AddMemberRequest{...}` passed to `s.members.addMemberToChat`. Nothing else in this file changes; `NewThreadPreviewService` and `GivePreview` stay as they are (the cache lives in `ThreadManagementService`, which `addMemberToChat` already uses).
13. **Test fakes** (`internal/service/thread_manager_add_member_test.go`):
    - Add `referenceStore store.ReferenceStore` to `fakeUnitOfWork` (lines 21-33) and `func (f fakeUnitOfWork) References() store.ReferenceStore` returning `&fakeReferenceStore{}` when nil (pattern: `ThreadPreviews()` at lines 150-156).
    - Add `type fakeReferenceStore struct { sources []*model.SourceRef; queues []*model.QueueRef; err error }` with recording `UpsertSource`/`UpsertQueue`.
14. **New tests** (new file `internal/service/reference_cache_test.go`, plus additions next to the AddMember/Transfer tests):
    - `addMemberToChat`/`AddMember` with `Queue: &model.QueueInfo{ID: 7, Name: "Sales"}` → fake reference store has one queue upsert with the resolved domain and `threadDialogStore.lastCreate.QueueID` points to 7. Model on existing AddMember tests in the same file (struct-literal service with `fakeUnitOfWork{threadDialogStore, messageStore, outboxStore}`; see `TestTransfer_AddsMemberRemovesInitiatorAndSendsTransferSystemMessage` at line 701 for a full transfer setup).
    - Without a queue → no upserts and `lastCreate.QueueID == nil`.
    - `Transfer` with a queue → `lastCreate.QueueID` = id and one queue upsert; without → nil.
    - `UpgradePreviewToFullMember` passes the queue through (see `internal/service/thread_preview_test.go` for how the preview service is built with `members`).
    - `touchSourceRef` with a nil cache always upserts; `sourceRefFromPeers` returns nil without via-info or with empty name.
    - **Source write path, new thread** (add to `internal/service/thread_manager_ensure_direct_test.go`, reuse its existing setup): `EnsureDirectThread` creating a thread where the external (non-bot) peer has `Identity.Via` + `Identity.ViaInfo{Name, Type}` → fake reference store records exactly one `UpsertSource` with `GateID == via`, the request `DomainID`, name and type, inside the creation transaction; after the tx succeeds the cache holds the value (a second identical `EnsureDirectThread` on the now-existing thread does no upsert).
    - **Source write path, existing thread**: `EnsureDirectThread` on an existing thread with via-info → one `UpsertSource` on a cache miss; with the same value already remembered → zero upserts; with a changed name → one upsert with the new name.
    - **No via-info (outbound / agent-originated or old providers)**: peers without `Identity.ViaInfo` (or with empty name) → zero `UpsertSource` calls for both new and existing threads, and thread creation behaves exactly as before.
    - **Rollback**: if the creation transaction fails after the source upsert (make a later fake store call return an error), the cache is NOT updated (remember func not called) — a retry upserts again.
    - Cache skip: a `ReferenceCache` built with an injected fake `cache.Cache` (implement the 5 methods with a map) returns `SourceChanged == false` for an equal value → no upsert.
15. Run `go build ./... && go vet ./... && go test ./internal/...`, then `golangci-lint run ./...` if available.

#### Files

- `migrations/20261015120000_create_source_queue_refs.sql` — **create** — goose migration from step 1; mirror `StatementBegin/End` and `if not exists` of `migrations/20261001120000_create_thread_preview_table.sql`.
- `migrations/20261001120000_create_thread_preview_table.sql` — **read** — style reference for the migration.
- `internal/domain/model/thread_ref.go` — **create** — `SourceRef`, `QueueRef`, `QueueInfo` (names used by subtask C).
- `internal/domain/model/thread_dialog.go` — **edit** — `QueueID *int64` on `ThreadDialogExtended` (struct lines 38-54, after `Via`).
- `internal/domain/shared/base.go` — **edit** — `ViaInfo` type, `Identity.ViaInfo` (struct lines 22-27), `Peer.ResolveViaInfo()` next to `ResolveVia` (lines 51-58).
- `internal/store/store.go` — **edit** — append `ReferenceStore` interface.
- `internal/store/uow.go` — **edit** — `References() ReferenceStore` on `UnitOfWork`.
- `internal/store/postgres/reference_store.go` — **create** — `referenceStore`; mirror constructor/assertion/NamedArgs/error style of `internal/store/postgres/thread_tag_store.go`.
- `internal/store/postgres/uow.go` — **edit** — field in struct (lines 14-35) + lazy `References()` getter mirroring `ThreadPreviews()` (168-174).
- `internal/store/postgres/thread_dialog_store.go` — **edit** — `Create` (48-136): `queue_id`/`@QueueID` in the CTE insert, `dial.via, dial.queue_id` in the select, NamedArg; `threadDialog` struct (26-46); `mapToThreadDialogExtendedModel` (139+).
- `internal/service/reference_cache.go` — **create** — `ReferenceCache`, `sourceRefFromPeers`, `touchSourceRef`, `touchQueueRef`, `queueIDOf`.
- `internal/service/module.go` — **edit** — add `NewReferenceCache` to `fx.Provide` (list at line 17).
- `internal/service/thread_manager.go` — **edit** — struct + `NewThreadService` (36-82), `addMemberToChat` (329-485), `Transfer` (487-682), `EnsureDirectThread` (1549-1582) + new `touchSourceForExistingThread`, `orchestrateDirectThreadCreation` (1666-1730), `initializeDirectThreadDialogs` (1788-1896).
- `internal/service/thread_preview.go` — **edit** — only `UpgradePreviewToFullMember` (174-214): `Queue: req.Queue` in the `dto.AddMemberRequest` literal (line ~187).
- `internal/service/dto/thread.go` — **edit** — `Queue *model.QueueInfo` on `AddMemberRequest` (83-94) and `TransferThreadRequest` (131-138).
- `internal/service/dto/thread_preview.go` — **edit** — `Queue *model.QueueInfo` on `UpgradePreviewRequest` (26-32) only.
- `internal/service/thread_manager_add_member_test.go` — **edit** — `References()` on `fakeUnitOfWork` (21-157), `fakeReferenceStore`, AddMember/Transfer queue tests (existing Transfer test at line 701).
- `internal/service/thread_manager_unread_test.go` — **edit** — line 55: trailing `nil` for the new `NewThreadService` parameter.
- `internal/service/thread_manager_tags_test.go` — **edit** — line 35: same.
- `internal/service/reference_cache_test.go` — **create** — cache/helper tests with a fake `cache.Cache`.
- `internal/service/thread_manager_ensure_direct_test.go` — **edit** — source write-path tests (new thread, existing thread hit/miss/changed, no via-info, rollback keeps cache clean).
- `infra/webitel/im-contact/client.go` — **read** — Ristretto build pattern (lines 79-111) and struct-wrapped cached values (`subCached`, `identityCached`).

#### Context

- **Lock order:** mutations lock thread → thread_dialog (`LockForUpdate` first). Upserting one ref row after the thread lock is fine; each tx touches at most one ref row, so no ref-vs-ref deadlocks.
- **Errors:** upsert errors inside a tx are returned (PG aborts the tx otherwise). Only the existing-thread refresh in `EnsureDirectThread` runs in its own tx and is therefore log-and-continue.
- **Bots:** `AddMember`/`Transfer` to bots normally carry no queue. Do not special-case; nil queue = no-op. If a queue is passed for a bot it is stored anyway.
- **Queue without name:** `ID > 0` with empty `Name` still sets `thread_dialog.queue_id` but skips the `queue_ref` upsert (name is `not null`); Search returns `queue` only when a non-deleted `queue_ref` row exists for that id (possibly from another call); otherwise `queue` is null (user decision: never return a queue without a ref).
- `extendedThreadDialogToSimpleMapper` (line 1778) does not need `QueueID`.
- `UpgradePreviewToFullMember` goes through `addMemberToChat`, so its queue reaches `thread_dialog.queue_id` automatically. The `beforeCreate` callback (preview revoke) runs before the queue upsert in the same tx; ordering is irrelevant.
- Existing tests in `thread_manager_ensure_direct_test.go`, `thread_preview_test.go`, `thread_manager_add_member_test.go`, `thread_manager_bot_control_test.go` (Transfer tests at 234 and 295) must keep passing. They use `fakeUnitOfWork` and struct-literal services with `refs == nil`, so every code path must tolerate a nil `*ReferenceCache` (nil cache → always upsert through the fake store, which is a no-op recorder).
- `ensure_direct` tests may build peers without `Identity.ViaInfo`; `sourceRefFromPeers` returns nil and nothing changes.
- Files owned by other subtasks — do not edit: `internal/store/query_object/**`, `internal/store/postgres/thread_store.go`, `internal/domain/model/thread.go`, `docs/thread_search.md` (B); `internal/handler/grpc/**`, `gen/go/**`, the protos repo (C).

### Subtask B: Search read path: lazy joins, sorting, defaults, read model and docs

**Blocked by:** Q8

#### Instructions

1. **Base query object** (`internal/store/query_object/query_object.go`):
   - Extend `fieldMetadata` (lines 20-26) with `nullsLast bool` (append ` NULLS LAST` in ORDER BY) and `sortOnly bool` (allowed in `WithSort`, never selected).
   - In `WithFields` (lines 47-64), skip entries whose `meta.sortOnly` is true (do not add them to `validFields`, do not call `EnsureJoins`).
   - In `ToSQL`, inside the select loop (lines 131-137) also skip `meta.sortOnly` (defensive). In the sort loop (lines 141-156), before `OrderBy` call `q.entity.EnsureJoins(meta.requiresJoin)` so late-decided joins are added, and build the clause as `fmt.Sprintf("%s %s", meta.sqlExpr, orderDir)` plus `" NULLS LAST"` when `meta.nullsLast`.
2. **Constants** (`internal/store/query_object/constants.go`, table const block lines 4-14): add `SourceRefTable string = "im_thread.source_ref"` and `QueueRefTable string = "im_thread.queue_ref"`.
3. **Thread query object** (`internal/store/query_object/thread.go`):
   - Aliases (const block lines 15-20): `threadSourceViaAlias = "srcv"`, `threadSourceRefAlias = "src"`, `threadQueueRefAlias = "qr"`.
   - Join bits (lines 22-29) after `threadLinkVariables`: `threadLinkSource`, `threadLinkQueue`.
   - Add `selfScoped bool` to `threadQueryObject` (lines 39-43). Set `q.selfScoped = true` next to each existing `q.mustIncludeComputedSubject = true` in `WithContactIDFilter` (lines 290-299, inside `len(memberIDs) != 0`), `WithSharedMembersFilter` (302-339, after the `selfID == uuid.Nil` guard) and `WithParticipantsFilter` (361+, after its guard). All three filter `td.member_id = selfID`, so `td` is then the caller's own row. Do not edit `thread_manager.go`.
   - `linkSource()` (guard + set bit, like `linkVariables` at lines 567-578), two `q.builder.LeftJoin(...)` calls:
     ```sql
     lateral (
       select sd.via as gate_id
       from im_thread.thread_dialog sd
       where sd.domain_id = t.domain_id and sd.thread_id = t.id
         and sd.is_bot = false and sd.via is not null and sd.via <> ''
       order by (sd.deleted_at is not null), sd.created_at
       limit 1
     ) srcv on true
     ```
     and `im_thread.source_ref src on src.gate_id = srcv.gate_id and src.deleted = false` (squirrel prepends `LEFT JOIN`).
   - `linkQueue()`: call `q.linkThreadDialog()` first, then `LeftJoin("im_thread.queue_ref qr on qr.domain_id = t.domain_id and qr.queue_id = td.queue_id and qr.deleted = false")`.
   - In `FieldsMetadata()` (lines 61-185) add closures: `resolveQueueJoin()` → `threadLinkQueue` if `q.selfScoped` else 0; queue select expr → scoped: `case when qr.queue_id is null then null else jsonb_build_object('id', qr.queue_id, 'name', qr.name) end as queue` (no `queue_ref` row → null queue, matching source / Q3), unscoped: `null::jsonb as queue`; queue sort expr → scoped `qr.name`, unscoped `null::text` (a bare `NULL` constant in ORDER BY is rejected). New entries:
     - `"source"`: `sqlExpr: "src.gate_id"`, `aliasedExpr: "case when src.gate_id is null then null else jsonb_build_object('id', src.gate_id, 'name', src.name, 'type', src.type) end as source"` (Q3: no ref → no source), `requiresJoin: threadLinkSource`, `sortable: false`.
     - `"source_name"`: `sqlExpr: "src.name"`, `requiresJoin: threadLinkSource`, `sortable: true`, `nullsLast: true`, `sortOnly: true`.
     - `"queue"`: aliased expr per above, `requiresJoin: resolveQueueJoin()`, `sortable: false`.
     - `"queue_name"`: `sqlExpr: <queue sort expr>`, `requiresJoin: resolveQueueJoin()`, `sortable: true`, `nullsLast: true`, `sortOnly: true`.
   - `EnsureJoins` (lines 187-207): handle both new bits.
   - **Defaults (Q7 + Q8):** `DefaultFields()` (lines 54-59).
     - Decided (Q8 = b): build the base slice and append `"source", "queue"` only `if q.selfScoped`. `ToSQL` (query_object.go:127-129) calls `DefaultFields()` after all filters, so Search (always self-scoped) gets them and Get / the internal Get at `thread_manager.go:1289` do not.
   - Why no Search reordering is needed: Search calls `WithFields`/`WithSort` before the self filters, so at that moment `resolveQueueJoin()` returns 0; `ToSQL` re-evaluates `FieldsMetadata()` per selected field and (after step 1) per sort field, when `selfScoped` is already true, so `EnsureJoins(threadLinkQueue)` runs after `td` is joined.
4. **Read model** (`internal/domain/model/thread.go`): add `type ThreadSource struct { ID string \`json:"id"\`; Name string \`json:"name"\`; Type string \`json:"type"\` }` and `type ThreadQueue struct { ID int64 \`json:"id"\`; Name string \`json:"name"\` }`; add `Source *ThreadSource \`json:"source,omitempty" db:"source"\`` and `Queue *ThreadQueue \`json:"queue,omitempty" db:"queue"\`` to `Thread` (struct lines 43-80, after `OwnerBotID`). JSON `null` name decodes to `""`. `ResolveThread` (`thread_store.go:~201`) scans into `model.Thread` with Lax and does not select these columns — fine.
5. **Store record** (`internal/store/postgres/thread_store.go`): add the same two fields (`*model.ThreadSource`, `*model.ThreadQueue`, tags `db:"source"` / `db:"queue"`) to `threadRecord` (lines 20-36) and copy them in `mapThreadRecordToModel` (150-187). pgx decodes jsonb into the struct pointer as for `LastMessage`/`Variables`; SQL NULL → nil. This is mandatory since both columns can now come from defaults (Lax fails on unknown columns).
6. **Tests** (`internal/store/query_object/thread_test.go`, use `assertSQLContains` at line 28; model on `TestThreadQuery_WithSearchFilter_MatchesSubjectMembersAndVariables`, line 44):
   - `NewThreadQueryObject().WithFields([]string{"id","source"}).ToSQL()` contains `from im_thread.thread_dialog sd`, `im_thread.source_ref src on src.gate_id = srcv.gate_id` and `as source`.
   - `WithFields([]string{"id"})` → SQL does not contain `source_ref`.
   - Search-like chain `WithSubject().WithFields([]string{"id","queue"}).WithSort("-queue_name").WithoutDeletedAtFilter().WithContactIDFilter(self)` → contains `im_thread.queue_ref qr`, `case when qr.queue_id is null then null`, `jsonb_build_object('id', qr.queue_id` and `ORDER BY qr.name DESC NULLS LAST`.
   - `WithSort("+source_name")` (fields without `source`) → `ORDER BY src.name ASC NULLS LAST` and the source joins present.
   - Unscoped `WithIDFilter(id).WithFields([]string{"queue"})` → `null::jsonb as queue`, no `queue_ref`.
   - `WithFields([]string{"source_name"})` → `source_name` not selected (falls back to defaults).
   - Defaults: `WithContactIDFilter(self)` with no fields → SQL contains `as source` and `as queue`. Also assert that `WithIDFilter(id)` with no fields contains neither `source_ref` nor `as queue` (Q8 = b).
   - Existing tests keep passing, including `paginator_test.go`, `search_left_test.go`, `direct_settings_test.go` (other entities share `baseQueryObject`; zero-valued new metadata keeps behaviour).
7. **Docs** (`docs/thread_search.md`): Section "1. Data Selection (Fields)" (line 7): describe `source` → `{id, name, type}` (gate id / provider display name / provider type such as `telegram_bot`; omitted when the gate is unknown) and `queue` → `{id, name}` of the call_center queue through which the requesting agent (`self_id`) received the thread; state that both are in the default set of Search only (Get does not include them by default, but `source` can be requested via fields). Section "3. Sorting" (line 18): add `source_name`, `queue_name`, NULLs last in both directions, sort-only. Add an eventual-consistency note: names refresh on inbound traffic / agent assignment; old threads may have empty values and sort last.
8. Run `go test ./internal/store/... ./internal/domain/...` and `go build ./...`.

#### Files

- `internal/store/query_object/query_object.go` — **edit** — `fieldMetadata` (20-26), `WithFields` (47-64), `ToSQL` select loop (131-137) and sort loop (141-156).
- `internal/store/query_object/constants.go` — **edit** — `SourceRefTable`/`QueueRefTable` in the table const block (4-14).
- `internal/store/query_object/thread.go` — **edit** — aliases (15-20), join bits (22-29), `selfScoped` field (39-43), `DefaultFields` (54-59), `FieldsMetadata` (61-185), `EnsureJoins` (187-207), flag in `WithContactIDFilter` (290-299), `WithSharedMembersFilter` (302-339), `WithParticipantsFilter` (361+), new `linkSource`/`linkQueue` modeled on `linkVariables` (567-578).
- `internal/store/query_object/thread_test.go` — **edit** — new tests with `assertSQLContains`.
- `internal/domain/model/thread.go` — **edit** — `ThreadSource`, `ThreadQueue`, `Thread.Source`/`Thread.Queue` (struct 43-80).
- `internal/store/postgres/thread_store.go` — **edit** — `threadRecord` (20-36), `mapThreadRecordToModel` (150-187).
- `docs/thread_search.md` — **edit** — Fields (line 7) and Sorting (line 18) sections + consistency note.
- `internal/service/thread_manager.go` — **read** — `Search` (124-170) call order `WithSubject → WithFields → … → WithSort → WithoutDeletedAtFilter → … → (WithSharedMembersFilter | WithContactIDFilter) → WithParticipantsFilter`; `Get` (92-121) and line 1289 use defaults. Do not edit (owned by A).

#### Context

- `baseQueryObject.ToSQL` calls `q.entity.FieldsMetadata()` fresh per field, so closures (like the existing `resolveSubjectJoin`) see the final `selfScoped`. Joins are deduplicated via the `q.join` bitmask; repeated `EnsureJoins` is safe.
- `selfScoped` is set in exactly the places that set `mustIncludeComputedSubject`; keep it a separate field for clarity.
- `ThreadStore.Get` (`thread_store.go:127-148`) uses `CollectExactlyOneRow`; joining `td` unscoped would multiply rows. The `selfScoped` gate prevents that for `queue`. `source` is a `limit 1` lateral and is always safe.
- `WithContactIDFilter` does not filter `td.deleted_at` (pre-existing); `queue` is read from whichever caller row the existing query already returns — no extra row multiplication is introduced.
- `SearchLeft` uses a different query object (`left_threads_message_history.go` / `search_left.go`) — out of scope.
- Offset pagination with a non-unique sort key can reorder ties between pages; pre-existing for `subject`, no tiebreaker.
- The tables queried here are created by subtask A's migration; B's unit tests need no DB. Do not touch A files (`migrations/**`, `internal/store/store.go`, `internal/store/uow.go`, `internal/store/postgres/uow.go`, `internal/store/postgres/thread_dialog_store.go`, `internal/service/**`, `internal/domain/shared/**`, `internal/domain/model/thread_dialog.go`, `internal/domain/model/thread_ref.go`) or C files (`internal/handler/grpc/**`, `gen/go/**`, protos repo).

### Subtask C: Thread-service protos, local gen/go regeneration and gRPC mappers

#### Instructions

Part 1 (protos + generation) does not depend on A/B and can start immediately. Part 2 (mappers) uses types from A and B; write it against the names below and run the final build/tests once A and B are in the tree.

1. **Protos** — in `/Users/dtsnk0/Documents/projects/protos` on the already checked-out branch `feat/thread-search-source-queue` (verify with `git -C /Users/dtsnk0/Documents/projects/protos branch --show-current`). Do NOT commit, do NOT push. Edit only `im/service/thread/v1/*`:
   - `peer.proto`: add after `Identity`:
     ```proto
     // ViaInfo is the display data of the gate identified by Identity.via,
     // sent by im-providers alongside the via header.
     message ViaInfo {
       // Gate display name.
       string name = 1;
       // Provider type, e.g. facebook | whatsapp | telegram_bot.
       string type = 2;
     }
     ```
     and in `Identity` add `// Display data of the gate in via. Optional.` + `ViaInfo via_info = 5;`.
   - `shared.proto`: add
     ```proto
     // ThreadQueue is a call_center queue through which an agent received a thread.
     message ThreadQueue {
       int64 id = 1;
       string name = 2;
     }
     ```
   - `thread_service.proto`:
     - Add `message ThreadSource { string id = 1; string name = 2; string type = 3; }` (comment: gate id = thread_dialog.via of the external member, gate name, provider type) near `Thread` in the `//#region Common` block.
     - In `message Thread` (lines 114-172) after `string updates_cursor = 20;`: `// Source gate of the thread (external member's via). Unset when unknown.` `ThreadSource source = 21;` and `// Queue through which the requesting participant (self_id) received the thread. Search only.` `ThreadQueue queue = 22;`.
     - `AddMemberRequest` (lines 408-439): after `optional bool system_call = 7;` add `// Optional call_center queue through which the new member receives the thread.` `ThreadQueue queue = 8;`.
     - `TransferRequest` (lines 461-489): after `optional bool auto_leave = 6;` add `// Optional call_center queue through which the target receives the thread. Empty → no queue.` `ThreadQueue queue = 7;`.
   - `thread_preview_service.proto`: in `UpgradePreviewToFullMemberRequest` (lines 60-66) add `ThreadQueue queue = 6;` with a comment. Do NOT change `GivePreviewRequest`.
   - Do not add `buf.validate` rules to the new fields (the service treats `id <= 0` as absent). Do not touch `im/api/gateway/**` or any other directory.
   - Validate: `cd /Users/dtsnk0/Documents/projects/protos/im && buf build && buf lint --path service/thread/v1` (if lint reports pre-existing issues in untouched lines, only ensure the new lines add none).
2. **Regenerate** `gen/go/thread/v1` from the local directory (template config stays untouched — the positional input overrides its `git_repo` input):
   ```sh
   cd /Users/dtsnk0/.superset/worktrees/im-thread-service/cedar-chips-97d85d4d/buf && \
   buf generate /Users/dtsnk0/Documents/projects/protos/im \
     --template buf.gen.thread.yaml \
     --path /Users/dtsnk0/Documents/projects/protos/im/service/thread/v1
   ```
   Then `git status gen buf Makefile` in im-thread-service: only `gen/go/thread/v1/*.go` may change; `buf/` and `Makefile` must be clean. Expect, besides the new fields, the upstream-only diffs `ThreadSearchRequest.Unread` and `DeletedMessage` (see Codebase notes) — keep them. Run `go build ./...` immediately: it must pass with the current code (all changes are additive).
3. Inspect the generated code and confirm names: `impb.ViaInfo` (`GetName()`, `GetType()`), `(*impb.Identity).GetViaInfo()`, `impb.ThreadSource` (`Id`, `Name`, `Type`), `impb.ThreadQueue` (`Id int64`, `Name`), `(*impb.Thread).Source/Queue`, `GetQueue()` on `AddMemberRequest`, `TransferRequest`, `UpgradePreviewToFullMemberRequest`. Adjust the mapper code below if generated names differ.
4. **Mappers** — add a small helper in `internal/handler/grpc/mapper/thread.go`:
   ```go
   func convertQueueInfo(q *impb.ThreadQueue) *model.QueueInfo {
       if q == nil || q.GetId() <= 0 { return nil }
       return &model.QueueInfo{ID: q.GetId(), Name: q.GetName()}
   }
   ```
   - `ConvertAddMemberRequest` (lines 126-160): `Queue: convertQueueInfo(in.GetQueue()),` in the `dto.AddMemberRequest` literal.
   - `ConvertTransferThreadRequest` (lines 162-190): `Queue: convertQueueInfo(in.GetQueue()),` in the `dto.TransferThreadRequest` literal.
   - `ConvertToThread` (lines 241-303): after the `OwnerBotID` block, `if source.Source != nil { thread.Source = &impb.ThreadSource{Id: source.Source.ID, Name: source.Source.Name, Type: source.Source.Type} }` and `if source.Queue != nil { thread.Queue = &impb.ThreadQueue{Id: source.Queue.ID, Name: source.Queue.Name} }`.
   - `internal/handler/grpc/mapper/thread_preview.go` `ConvertUpgradePreviewRequest` (lines 69-94): `Queue: convertQueueInfo(in.GetQueue()),` (same package, helper is visible). `ConvertGivePreviewRequest` stays unchanged.
   - `internal/handler/grpc/mapper/text_message.go` `MapPeerFromProto` (lines 56-89), `Peer_ContactId` branch, after building `p.Identity` (line 69-74): `if vi := id.GetViaInfo(); vi != nil { p.Identity.ViaInfo = &shared.ViaInfo{Name: vi.GetName(), Type: vi.GetType()} }`.
5. **Mapper tests** — add to `internal/handler/grpc/mapper/thread_test.go` (next to `TestConvertGivePreviewRequest_ValidRequest`, line 88; plain `testing` + `require`, see the file's imports):
   - `MapPeerFromProto` with `Identity{Via: &via, ViaInfo: &impb.ViaInfo{Name:"Bot", Type:"telegram_bot"}}` → `p.Identity.ViaInfo` set; without → nil.
   - `ConvertAddMemberRequest`, `ConvertTransferThreadRequest`, `ConvertUpgradePreviewRequest` with `Queue{Id:7, Name:"Sales"}` → `Queue == &model.QueueInfo{7,"Sales"}`; with no queue or `Id: 0` → nil. Use valid UUID strings for required ids.
   - `(&ThreadOutConverter{}).ConvertToThread(&model.Thread{ID: uuid.New(), Source: &model.ThreadSource{…}, Queue: &model.ThreadQueue{…}})` maps both; nil model fields → nil proto fields.
6. Once A and B are present: `go build ./... && go vet ./... && go test ./internal/handler/...` (and full `go test ./...`).

#### Files

- `/Users/dtsnk0/Documents/projects/protos/im/service/thread/v1/peer.proto` — **edit** — `ViaInfo` message and `Identity.via_info = 5` (Identity at lines 8-16, fields 1-4 used).
- `/Users/dtsnk0/Documents/projects/protos/im/service/thread/v1/shared.proto` — **edit** — `ThreadQueue` message (file has `ThreadRole`, `MessageDeliveryStatus`, `ThreadMember`; imported by both thread_service and thread_preview_service).
- `/Users/dtsnk0/Documents/projects/protos/im/service/thread/v1/thread_service.proto` — **edit** — `ThreadSource`, `Thread.source = 21`/`queue = 22` (Thread lines 114-172, last field `updates_cursor = 20`, 19 reserved), `AddMemberRequest.queue = 8` (408-439), `TransferRequest.queue = 7` (461-489).
- `/Users/dtsnk0/Documents/projects/protos/im/service/thread/v1/thread_preview_service.proto` — **edit** — `UpgradePreviewToFullMemberRequest.queue = 6` (lines 60-66).
- `gen/go/thread/v1/peer.pb.go`, `gen/go/thread/v1/shared.pb.go`, `gen/go/thread/v1/thread_service.pb.go`, `gen/go/thread/v1/thread_preview_service.pb.go` (+ `updates_service.pb.go` upstream diff) — **edit (regenerate only, never hand-edit)** — via the step 2 command.
- `buf/buf.gen.thread.yaml` — **read** — template used as-is; `inputs` points at protos `main`, `out: ../gen/go` relative to `buf/`. Must stay unchanged.
- `internal/handler/grpc/mapper/text_message.go` — **edit** — `MapPeerFromProto` (56-89).
- `internal/handler/grpc/mapper/thread.go` — **edit** — `convertQueueInfo` helper, `ConvertAddMemberRequest` (126-160), `ConvertTransferThreadRequest` (162-190), `ConvertToThread` (241-303).
- `internal/handler/grpc/mapper/thread_preview.go` — **edit** — `ConvertUpgradePreviewRequest` (69-94).
- `internal/handler/grpc/mapper/thread_test.go` — **edit** — new mapper tests (existing tests at lines 17-111).

#### Context

- Type names provided by the other subtasks (code against them; the package compiles only when they exist):
  - A: `model.QueueInfo{ID int64; Name string}` (`internal/domain/model/thread_ref.go`), `shared.ViaInfo{Name, Type string}` and `shared.Identity.ViaInfo *ViaInfo` (`internal/domain/shared/base.go`), `Queue *model.QueueInfo` on `dto.AddMemberRequest`, `dto.TransferThreadRequest`, `dto.UpgradePreviewRequest`.
  - B: `model.ThreadSource{ID, Name, Type string}`, `model.ThreadQueue{ID int64; Name string}`, `model.Thread.Source *ThreadSource`, `model.Thread.Queue *ThreadQueue` (`internal/domain/model/thread.go`).
- The handlers in `internal/handler/grpc/thread.go` (Transfer at 159-180, AddMember) and `thread_preview.go` need no change: they pass converted DTOs through and return `ConvertToThread` output.
- Protos `go_package` options in the `.proto` files are overridden by the template (`managed.override go_package`), so the generated package stays `thread` in `gen/go/thread/v1` regardless of the per-file `option go_package` lines — do not edit those options.
- `thread_service.proto` already imports `service/thread/v1/peer.proto` and `shared.proto`; `thread_preview_service.proto` imports `shared.proto`, which is why `ThreadQueue` lives in `shared.proto`. No new imports are needed.
- Before merge (not part of this subtask's run): once the protos PR is merged to `main`, regenerate with the committed template from protos `main` (e.g. the same `buf generate` with the git input, i.e. without the positional local path) so `gen/go` matches upstream.
- Do not edit files owned by A or B, `GivePreviewRequest`, `ConvertGivePreviewRequest`, or anything under `im/api/gateway` in protos.
