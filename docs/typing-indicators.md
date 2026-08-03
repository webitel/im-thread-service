# Typing Indicators & Live Typing Preview

Ephemeral "…is typing" signals between thread participants, plus an optional
live preview of the client's unsent draft for operators. Both are **real-time
only**: shown to currently-online participants, never stored in history, never
pushed, never replayed on reconnect. Modelled on the Telegram typing model
(client shows the indicator for a timeout and self-expires it).

Ownership is split:

- **im-thread-service** (this repo) owns the write side: the `SendTyping` RPC,
  membership/role resolution, rate-limiting, and the **ephemeral publish** to
  RabbitMQ that bypasses the transactional outbox. It also fans typing out to
  external channels via im-providers.
- **im-delivery-service** relays the event to online sessions only.
- **im-providers-service** forwards a native typing action to external channels
  that support it (Meta `sender_action`, Telegram `sendChatAction`).

## Key decision: ephemeral publish bypasses the outbox

Every other event in this service goes Postgres outbox → forwarder → RabbitMQ
(guaranteed, but a DB write per event). Typing is worthless within seconds and
streamed continuously, so it takes a **direct fire-and-forget publish** through
the existing RabbitMQ publisher (`internal/adapter/pubsub/publisher.go`,
exchange `im_message.events`, topic) with routing key
`im_message.<thread_id>.typing.v1`. No outbox row, no transaction, no delivery
guarantee — a lost typing event breaks nothing.

The server holds **no** typing state: the event carries `timeout_ms` and the
client self-expires the indicator. This removes a whole class of
cleanup-on-crash problems.

## The RPC

`webitel.im.service.thread.v1.Message/SendTyping` — one RPC for humans and bots:

```proto
message SendTypingRequest {
  Peer   from         = 1;   // who is typing
  Peer   to           = 2;   // target thread (Peer.thread_id)
  int64  domain_id    = 3;
  optional int32  timeout_ms   = 4;  // default 6000, clamped to 30000
  optional string preview_text = 5;  // ≤1 KiB; empty = clear preview
}
message SendTypingResponse {}   // empty: fire-and-forget
```

Flow (`internal/service/typing.go`):

1. Validate (`guards.SendTypingGuard`), resolve `thread_id` from the `to` peer,
   clamp `timeout_ms`, truncate `preview_text` to 1 KiB on a rune boundary.
2. Feature master switch (`typing.enabled`); if off → accept-and-ignore.
3. **Rate-limit** per participant per thread (Redis, best-effort, fail-open):
   `internal/adapter/ratelimit/cache.go`, key `typing:rl:<thread>:<member>`,
   window `typing.rate_limit_window` (3 s). Excess events are dropped silently.
4. Membership check: the sender must be an active member of the thread
   (`ThreadDialogStore.GetQuickView`, domain-scoped).
5. Resolve recipients + preview allow-list (below).
6. Ephemeral publish of `event.Typing`; then best-effort outbound to external
   peers via im-providers.

## Recipients and the preview allow-list

The loaded members drive two sets:

- **`to`** (indicator recipients) — internal participants: `Via == nil`,
  `!IsBot`, not the sender. External peers reach their channel via im-providers,
  not the stream; offline members are dropped by im-delivery.
- **`preview_visible_to`** (draft allow-list) — currently the same internal set;
  the draft is attached only when `typing.preview_enabled` is on **and** the set
  is non-empty. Isolated in `internalRecipients`/`isInternalRecipient` so the
  exact operator-vs-customer predicate can be tightened after product signoff.

### Privacy (Live Typing Preview)

`preview_text` is the client's **unsent** draft — the most sensitive data in a
chat. Guarantees:

- **Off by default** (`typing.preview_enabled = false`) pending product/legal
  signoff.
- **Never logged**: `event.Typing` implements `slog.LogValuer` and emits only
  `preview_bytes=<n>`, never the text.
- **Never persisted**: it lives only in the ephemeral event, no Postgres/Redis.
- **Role-gated**: attached only for the `preview_visible_to` allow-list; the
  list itself is server-only and is not forwarded to clients by im-delivery.
- Size-capped (`typing.max_preview_bytes`, 1 KiB) with rune-safe truncation.

## The ephemeral event

`internal/domain/event/typing_v1.go` — a plain JSON struct (NOT an `Outboxer`):

```jsonc
{
  "thread_id": "…",
  "member_id": "…",            // who is typing
  "timeout_ms": 6000,
  "occurred_at": "…",
  "to": ["…", "…"],            // internal recipient member ids
  "preview_text": "…",         // only when preview enabled + allowed recipients
  "preview_visible_to": ["…"]  // subset of `to` allowed to see the draft
}
```

Routing key: `im_message.<thread_id>.typing.v1` on `im_message.events`.

## Outbound to external channels

For each external peer (`Via != nil`, not the sender), im-thread calls
`im-providers Mark... SendTyping` best-effort (`ProvidersAdapter.SendTyping`,
`internal/service/providers_adapter.go`). im-providers maps it to the channel's
native action and no-ops for channels without a `TypingSender` (capability
`supports_typing`). Telegram provider is not implemented yet — that path is a
no-op until it lands.

## Config (`config/config.go`, `TypingConfig`)

| Key | Default | Meaning |
|-----|---------|---------|
| `typing.enabled` | `true` | master switch |
| `typing.preview_enabled` | **`false`** | Live Typing Preview (privacy-gated) |
| `typing.rate_limit_window` | `3s` | min interval per participant per thread |
| `typing.default_timeout` | `6s` | indicator lifetime when unset |
| `typing.max_timeout` | `30s` | upper clamp (bots) |
| `typing.max_preview_bytes` | `1024` | preview cap |

## Client contract (via im-delivery)

Delivery relays the event as a `TypingEvent` to **online** sessions only
(`thread_id`, `member_id`, `timeout_ms`, `preview_text` for authorized sessions).
Clients show "X is typing…" for `timeout_ms` and self-expire; the indicator is
also cleared when a `MessageCreated` from the same member arrives. No explicit
`typing_stop` (TTL-only, per design).

## Changes by service

**protos/im** — `SendTyping` RPC + messages (thread `message_service.proto`);
`TypingEvent` in `api/delivery/v1/delivery.proto`; `SendTyping` +
`supports_typing` capability in `service/provider/v1`.

**im-thread-service** — `SendTyping` service/handler/mapper/dto/guard; ephemeral
`event.Typing`; `internal/adapter/ratelimit`; Redis + cache providers; providers
adapter `SendTyping`; `TypingConfig`.

**im-delivery-service** — consume `im_message.#.typing.v1`, emit `TypingEvent` to
online sessions only, per-session preview enforcement (see the delivery
`events.md`).

**im-providers-service** — `SendTyping` RPC + `TypingSender` adapters +
`supports_typing` capability.

## Testing

- `internal/service/typing_test.go` — feature-off no-op, rate-limit drop,
  non-member rejection, plain-typing topic, preview role gating (bot/external
  excluded), rune-safe truncation, timeout clamp, external-peer dispatch to
  providers.
- `internal/adapter/ratelimit` — first-in-window allowed / second dropped /
  fail-open on cache error.
- `go build`, `go vet`, `go test ./...` pass in im-thread-service; `buf
  breaking` green.

## Follow-ups

- Tighten the operator-vs-customer predicate for `preview_visible_to` after
  product signoff (one-directional client→operator preview).
- Telegram outbound typing (no provider package yet).
- Product/legal signoff before enabling `typing.preview_enabled`.
