# Message Reactions (emoji)

Client and operator can leave a single emoji reaction on any message, and the
reaction is shown to both sides. Modelled on the canonical messenger design —
Telegram's server-controlled `availableReactions` plus a reaction that is a
tagged union (`reactionEmoji` / `reactionCustomEmoji`), WhatsApp's one-reaction
-per-user with replace/toggle — a **single reaction row per (message, reactor)**
with an **idempotency ledger** and an **aggregated read model**.

## The model

A reaction is at most **one per member per message**. The write API is a single
`Message.SetReaction` RPC with toggle/replace/clear semantics:

```
set 👍            -> 👍
set ❤️ (had 👍)    -> ❤️        (replace)
set ❤️ (had ❤️)    -> · removed  (toggle off)
set "" (empty)     -> · removed  (clear)
```

The call returns the action it took: `SET`, `REMOVED`, or `UNCHANGED`.

### Reaction content is a union

`ReactionContent` is a `oneof` so the contract can grow to custom
(sticker-backed) emoji without a breaking change:

```proto
message ReactionContent {
  oneof kind {
    string emoji           = 1;  // unicode; validated server-side
    string custom_emoji_id = 2;  // reserved — rejected today
  }
}
```

Only the unicode `emoji` arm is implemented. `custom_emoji_id` (the bridge to
stickers / Telegram premium custom emoji) is defined but rejected by the guard —
see *Not in this change*.

### Hybrid validation

Input is validated in `internal/service/guards/set_reaction.go`, then a per-gate
allow-list is applied at forward time (below):

| Stage | Rule |
|-------|------|
| custom arm | `custom_emoji_id` set → `InvalidArgument` (`…set_reaction.custom_unsupported`) |
| empty | `emoji == ""` is a clear — allowed, no further checks |
| normalize | NFC (`golang.org/x/text/unicode/norm`) — stored canonical so equal emoji dedupe |
| single grapheme | exactly one grapheme cluster (`rivo/uniseg`) — a family/ZWJ/skin-tone/flag emoji is still **one** cluster |
| is-emoji | the cluster is a real emoji (`forPelevin/gomoji`) — rejects letters, digits, CJK |

`❌ non-emoji` and `❌ multi-grapheme` return distinct error ids so the client can
message precisely.

## How the toggle is stored (idempotent, one statement)

`internal/store/postgres/message_reaction_store.go` settles a reaction in a
single round-trip. The statement guards, deduplicates and mutates together so a
partial state is impossible:

```
guard  -> reactor's active dialog exists AND coalesce(tp.can_react_messages, true)
          AND the message exists in the domain and is not deleted
claim  -> INSERT send_id INTO message_reaction_dedup ON CONFLICT DO NOTHING
          (a send_id already present => this exact request was applied before)
del    -> DELETE the row when emoji is empty OR repeats the stored one (toggle off)
ups    -> INSERT/REPLACE the row for any other non-empty emoji
```

`del` and `ups` are mutually exclusive by construction, so they never touch the
same row within the statement. The final `SELECT` returns `allowed`, the resolved
`thread_id`, the `action`, the stored `emoji`, and `reacted_at`. Membership /
permission failure surfaces as `allowed = false` → `store.ErrReactionNotAllowed`
→ `Forbidden` at the service (`…set_reaction.not_allowed`).

`UNIQUE(message_id, reactor_id)` is the source of truth for "one reaction per
user"; concurrent distinct-emoji writes serialize on it.

### Idempotency lives in a ledger, not on the row

The dedup key is stored in a separate `im_message.message_reaction_dedup`
`(message_id, reactor_id, send_id)` table, **not** on the reaction row. This is
deliberate: an earlier design kept `last_send_id` on the reaction row, but a
toggle-off **deletes** the row, so a redelivered toggle-off then re-created the
reaction. Keeping the consumed `send_id` off-row makes every at-least-once
redelivery — set, replace **or** remove — a true no-op, and stops a reordered
older retry from reverting a newer state.

Distinct requests still apply in **arrival order** (last-writer-wins) — this is
documented, not "monotonic": `send_id` is an opaque idempotency key, not a
logical clock.

## Read model — aggregated per emoji

History exposes reactions **grouped by emoji**, the shape a UI renders directly
(`👍 3`, `❤️ 1`, with the caller highlighted). The `v_messages` view aggregates:

```sql
( SELECT jsonb_agg(jsonb_build_object(
      'emoji', e.emoji, 'count', e.cnt,
      'reactor_ids', e.reactor_ids, 'last_reacted_at', e.last_ms
   ) ORDER BY e.first_at)
   FROM ( SELECT mr.emoji,
                 count(*)::int AS cnt,
                 to_jsonb((array_agg(mr.reactor_id ORDER BY mr.created_at))[1:12]) AS reactor_ids,
                 min(mr.created_at) AS first_at,
                 (extract(epoch from max(mr.updated_at)) * 1000)::bigint AS last_ms
            FROM im_message.message_reactions mr
           WHERE mr.message_id = m.id
           GROUP BY mr.emoji ) e ) AS reactions
```

`reacted_by_me` is **not** stored — it is derived per request from the history
`caller_id` (`MessageReaction.ReactedBy(callerID)` over the sampled reactor set).
The reactor sample is capped (`[1:12]`) while `count` is exact.

```
HistoryMessage.reactions[] = { reaction{emoji}, count, reacted_by_me, reactor_ids[], last_reacted_at }
```

## Real-time — shown to both sides

A change publishes `im.message.reaction` (v1) through the transactional outbox to
`im_message.<thread>.message.reaction.v1`; an `UNCHANGED` no-op publishes
nothing. im-delivery consumes it, enriches the reactor, and pushes
`ServerEvent.message_reaction_event` to every online session — the reactor's echo
included — so both sides converge. Only unicode flows on this path today.

```
SetReaction (changed) -> outbox im.message.reaction -> im-delivery
  -> ServerEvent.message_reaction_event -> ws / long-poll / grpc (both parties)
load/reconnect -> HistoryMessage.reactions (authoritative, aggregated)
```

## External messenger — best-effort, capability-gated

After commit, thread forwards the change to im-providers
(`ProvidersAdapter.SendReaction`). It is best-effort and messenger-dependent:

- the target message's platform id is resolved from the external-id mapping per
  gate; a gate with no mapping is skipped;
- a **set** is gated by the gate's reaction allow-list
  (`ReactionCapabilities`, config `reactions.allowed_emoji`; empty = unrestricted);
  a **removal** is always forwarded (clearing must reach the far side);
- im-providers dispatches to the driver via an optional `ReactionSender`
  interface — **Telegram** maps it to `setMessageReaction`; providers that don't
  implement it are a transparent no-op.

## Permissions

`thread_permission.can_react_messages` (default `true`, revocable per member,
mirrors `can_delete_messages`). The store gate uses
`coalesce(tp.can_react_messages, true)`, so dialogs predating the column keep the
default. The permission is threaded through the `ThreadPermissionManagement`
Get/Update surface like the other flags.

## API surface

- **`Message.SetReaction(SetReactionRequest) → SetReactionResponse`** — reactor
  (Peer), message_id, `ReactionContent reaction` (empty/unset clears), domain_id,
  optional thread_id, `send_id` (idempotency), external_id (inbound relays).
- **Gateway** — `POST /v1/messages/{message_id}/reaction`, body `{ emoji, send_id }`;
  the reactor comes from the authenticated identity and domain_id from context.
  A plain `emoji` string is exposed (no custom emoji); the gateway wraps it into
  the thread `ReactionContent`.
- **`HistoryMessage.reactions`** — aggregated read model above.

## Migration

`migrations/20260731120000_add_message_reactions.sql`:

1. `im_message.message_reactions` — `UNIQUE(message_id, reactor_id)`, index on `message_id`.
2. `im_message.message_reaction_dedup` — idempotency ledger `(message_id, reactor_id, send_id)` + `created_at` index (bounded state; purge periodically).
3. `thread_permission.can_react_messages BOOLEAN NOT NULL DEFAULT TRUE`.
4. Rebuild `im_thread.v_messages` with the aggregated `reactions` column. Down restores the prior (soft-delete) view and drops the tables/column.

## Changes by service

**protos/im** — additive, non-breaking (`buf breaking` clean):
- thread: `SetReaction` RPC, `ReactionContent`, `ReactionAction`, aggregated
  `MessageReaction`, `HistoryMessage.reactions`, `can_react_messages`.
- delivery: `ServerEvent.message_reaction_event` + `MessageReactionEvent`.
- provider: `SendReaction` + `ProviderSendReaction{Request,Response}`.
- gateway: client-facing `SetReaction` + REST route.

**im-thread-service** — migration; `Reaction`/`ReactionResult`/`MessageReaction`
domain + `MessageReaction` event (`im.message.reaction`); `MessageReactionStore`
(single-statement toggle) + uow wiring; `MessageService.SetReaction` + guard +
`ReactionCapabilities`; provider forward; handler/mapper/dto; aggregated history
read side; `can_react_messages` through the permission layer.

**im-delivery-service** — reaction payload + domain model + `EventKind` + factory +
listener + router binding (`im_message.#.message.reaction.v1`) + ws/long-poll/grpc
marshallers (mirrors `message.deleted`).

**im-providers-service** — `SendReaction` handler + optional `ReactionSender`
interface; Telegram `setMessageReaction` client + driver; Facebook/WhatsApp no-op.

**im-gateway-service** — client-facing `SetReaction` proxy (handler + service +
thread client wrapper).

## Not in this change / follow-ups

- **Custom emoji / stickers / GIFs** — `custom_emoji_id` is reserved in the union
  but rejected; stickers and GIFs are a separate message type, not reactions.
- **Per-gate capability source** — `ReactionCapabilities` is config-backed and
  gate-agnostic today; a real per-messenger allow-list from im-providers is the
  seam (the `gateID` argument) for a follow-up.
- **Dedup ledger TTL** — a periodic purge of old `message_reaction_dedup` rows is
  not wired to a cron yet.
- **Facebook/WhatsApp reaction forward** — only Telegram implements the driver;
  others are transparent no-ops.

## Testing

- Guard table tests (`internal/service/guards/set_reaction_test.go`): single
  emoji, skin-tone / ZWJ-family / flag pass; multi-grapheme, non-emoji and custom
  arm rejected with their ids; empty = clear.
- Service tests (`internal/service/message_reaction_test.go`): a changed reaction
  emits exactly one `im.message.reaction` event carrying reactor + recipients and
  is forwarded; `UNCHANGED` publishes nothing and is not forwarded; store
  rejection → `Forbidden`; missing ids rejected before the store.
- Mapper tests (`internal/handler/grpc/mapper/set_reaction_test.go`): request/
  response `ReactionContent` mapping, action-enum mapping, aggregated reactions +
  `reacted_by_me`.
- `go build`, `go vet`, `golangci-lint`, `go test ./...` pass in im-thread-service;
  build/lint/tests verified in im-delivery, im-gateway, im-providers.
- The store SQL (toggle, dedup, aggregation view) has no DB-backed test — the repo
  has no integration harness, consistent with the rest of the SQL layer; verify on
  dev with the flow in the PR description.
