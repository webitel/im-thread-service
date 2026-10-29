# Source and Queue in SearchLeft (left threads)

`ThreadManagement.SearchLeft` lists the threads the requesting agent has left.
Each item is one closed membership period. It now returns the same `source`
and `queue` objects as `Search` (see `docs/thread-search-source-queue.md`). Both
are **default fields**, so they come back when `fields` is empty:

- `source {id, name, type}`: the thread's provider gate. This is thread-level
  and uses the same SQL as `Search`/`Get`.
- `queue {id, name}`: the call_center queue through which the agent received
  the thread **in that membership period**.

The `source_name` and `queue_name` sort keys work in `SearchLeft` too.

Plan: `docs/dev-task-plans/abandoned-threads-queue-source.md` (local only,
gitignored). Client-facing API reference: `docs/thread_search.md`.

## Why

The active-chat list (`Search`) already shows and sorts by source and queue.
The "left / closed chats" list (`SearchLeft`) did not, so the client could not
show where a closed conversation came from or which queue delivered it.
"Abandoned threads" in the task means `SearchLeft`; the code never uses the
word "abandon".

## What changed

Only the query object changed. Proto (`Thread.source = 21`, `queue = 22`), the
store (`threadRecord.Source/Queue`, `db:"source"/"queue"`), the gRPC mapper
(`ConvertToThread`), the service and im-gateway-service already carried these
fields.

### `internal/store/query_object/thread.go`

- New package-level `sourceJoinClauses(threadTableAlias) (viaLateral, refJoin)`
  holds the source lateral (`srcv`) and the `im_thread.source_ref src` join.
  `threadQueryObject.linkSource` now calls it, and the SQL for
  `Search`/`Get`/bot-control is unchanged.
- New const `sourceSelectExpr` holds the `case when srcv.gate_id is null then
  null else jsonb_build_object(...) end as source` expression. Both query
  objects use it.

### `internal/store/query_object/search_left.go`

- The `membership_periods` CTE now selects `queue_id`.
- New join bits: `searchLeftLinkSource`, `searchLeftLinkQueue`.
- `DefaultFields()` gains `"source", "queue"`.
- `FieldsMetadata()` gains four entries:

  | key          | select                         | order by        | join bit               | notes                         |
  |--------------|--------------------------------|-----------------|------------------------|-------------------------------|
  | `source`     | `sourceSelectExpr`             | –               | `searchLeftLinkSource` | NULL when no member has a gate |
  | `source_name`| –                              | `src.name`      | `searchLeftLinkSource` | sort-only, `NULLS LAST`       |
  | `queue`      | `case when qr.queue_id is null then null else jsonb_build_object('id', qr.queue_id, 'name', qr.name) end` | – | `searchLeftLinkQueue` | never `{id, name: ""}` |
  | `queue_name` | –                              | `qr.name`       | `searchLeftLinkQueue`  | sort-only, `NULLS LAST`       |

- `EnsureJoins` calls the new idempotent `linkSource()` / `linkQueue()`:
  - `linkSource`: `sourceJoinClauses(threadAlias)`, which gives the same lateral
    as Search (first non-bot `thread_dialog` row with a non-empty `via`, active
    rows first, then `created_at`, `limit 1`).
  - `linkQueue`: `left join im_thread.queue_ref qr on qr.domain_id =
    t.domain_id and qr.queue_id = lm.queue_id and qr.deleted = false`.
- `ToSQL` is unchanged. `lm.left_at DESC` is added only when no sort is given.

### Why `queue` comes from `lm.queue_id`

Each `SearchLeft` row already is the agent's own `thread_dialog` row for that
period (the item `id` is that row id; the thread id is in `thread_ref_id`).
`thread_dialog.queue_id` is written once on insert, and leaving only sets
`deleted_at` / `leave_reason`. So the left row still holds the queue the agent
received the thread through. An agent with several periods in one thread gets
each period's own queue. The CTE has no `domain_id`, so the domain is taken
from `t.domain_id`.

### Row safety and laziness

- `queue_ref` is joined on its PK `(domain_id, queue_id)`. The source lateral
  has `limit 1` and joins `source_ref` on its PK `gate_id`. Neither join adds
  rows.
- Joins are added only when a field or sort key needs them. With an explicit
  `fields` list that has no `source`, `queue`, `source_name` or `queue_name`,
  the SQL has neither `source_ref` nor `queue_ref`. With an empty `fields`
  list, both joins are added because the fields are defaults.

### Docs

- `docs/thread_search.md`: a SearchLeft note in "1. Data Selection". The
  default-set line now says "Search and SearchLeft (not `Get`)".
- `docs/thread-search-source-queue.md`: a SearchLeft bullet under "Read path".

## Acceptance criteria

| # | Criterion | Met |
|---|-----------|-----|
| 1 | Empty `fields` returns `source` and `queue`; both are in `DefaultFields()` | yes |
| 2 | `source` uses the same lateral and `source_ref` join as Search/Get; NULL without a gate | yes (shared `sourceJoinClauses` / `sourceSelectExpr`) |
| 3 | `queue` is the period's own `lm.queue_id`, joined on `qr.domain_id = t.domain_id`; NULL, never `{id, name: ""}` | yes |
| 4 | `membership_periods` CTE selects `queue_id` | yes |
| 5 | `±source_name` → `src.name`, `±queue_name` → `qr.name`, `NULLS LAST`, sort-only; no default `lm.left_at DESC` when sorted | yes |
| 6 | Joins stay lazy; no `source_ref`/`queue_ref` when fields exclude them | yes |
| 7 | No row multiplication (PK joins, `limit 1`) | yes |
| 8 | `Search`/`Get`/bot-control SQL unchanged; `TestThreadQuery_*` pass unchanged | yes (`thread_test.go` untouched) |
| 9 | New unit tests cover defaults, source join, queue join, CTE, sorts, sort-only, lazy joins | yes (8 tests) |
| 10 | `go build`, `go vet`, `go test ./...` pass; `golangci-lint` shows no new issues | yes (0 issues) |
| 11 | `docs/thread_search.md` and `docs/thread-search-source-queue.md` updated | yes |

## Open issues from review

All are low severity and none blocks the merge.

1. **The queue expression and join are duplicated.** The `case when
   qr.queue_id is null ... end as queue` expression in `search_left.go` is a
   copy of `resolveQueueFieldExpression` in `thread.go`. The `queue_ref` join
   also repeats `threadQueryObject.linkQueue`; only the right-hand side
   (`lm.queue_id`) differs. Source has a shared helper (`sourceJoinClauses` /
   `sourceSelectExpr`), but queue does not. If one copy changes and the other
   does not, Search and SearchLeft will return different data. Suggested fix:
   add a shared `queueSelectExpr` const and a `queueRefJoin(threadAlias,
   queueIDExpr)` helper.
2. **The "Search only" wording in `docs/thread_search.md` was stale.** It is
   fixed in this doc pass: the line now says "Search and SearchLeft (not
   `Get`)".
3. **`TestSearchLeftQuery_CTESelectsQueueID` is brittle.** It matches the exact
   fragment `deleted_at AS left_at, queue_id`. Reordering the CTE columns would
   fail the test even though the behaviour stays the same.

Inherited caveats from the Search work still apply. `source_ref` is joined by
`gate_id` without a `domain_id` predicate, and a `±queue_name` /
`±source_name` sort has no tie-breaker.

## How to use

The request is unchanged:

```jsonc
// SearchLeftRequest
{
  "member_id": "<agent member uuid>",
  "fields": [],                // empty -> defaults, now including source and queue
  "sort": "-queue_name",       // or "+source_name"; NULLS LAST either way
  "size": 20, "page": 1
}
```

Each `items[]` entry may now carry:

```jsonc
"source": { "id": "<gate id>", "name": "Support bot", "type": "telegram_bot" },
"queue":  { "id": 42, "name": "Sales" }   // queue of this membership period
```

To skip the extra joins, pass an explicit `fields` list without
`source` / `queue`.

## How to verify

```sh
go test ./internal/store/query_object/ -run 'SearchLeft|Source|Queue'
go build ./... && go vet ./... && go test ./...
golangci-lint run ./internal/store/query_object/...
```

The new tests are in `internal/store/query_object/search_left_test.go`:
`Defaults_IncludeSourceAndQueue`, `CTESelectsQueueID`,
`Source_JoinsSourceRef`, `Queue_FromMembershipPeriod`,
`WithoutSourceAndQueue_NoRefJoins`, `SortQueueNameNullsLast`,
`SortSourceNameNullsLast`, `SortOnlyFields_NotSelected`.

To check manually against a DB with the source/queue ref migration applied:

1. Assign an agent to a thread through a queue (`AddMember` with `queue`), then
   have the agent leave it.
2. Call `SearchLeft` for that agent with empty `fields`. The item has `queue`
   for that queue and `source` for the thread's gate.
3. Re-assign the same agent through a different queue, then leave again.
   `SearchLeft` returns two items for the thread, each with its own queue.
4. Soft-delete the `queue_ref` row. `queue` is omitted for that item and is
   never `{id, name: ""}`.
