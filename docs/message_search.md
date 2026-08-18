# Message Search Endpoint Documentation

`MessageHistory.SearchMessages` finds messages by their text. It answers both
"search this dialog" and "search everything I can read", and every hit carries
the thread it belongs to so a client can open that dialog and jump to the
message.

Exposed to clients by im-gateway-service as `GET /v1/messages/search`.

## Request (SearchMessagesRequest)

*   **`q`** (string, required, 1..256): the term. Matched case-insensitively
    against any part of the message body (`ilike '%term%'`). Wildcards typed by
    the user (`%`, `_`) are escaped and matched literally.
*   **`thread_id`** (UUID, optional): narrows the search to one dialog. Empty
    spans every thread the caller belongs to.
*   **`caller_id`** (UUID, required): the member running the search. Set by the
    gateway from the authenticated identity — clients never pass it.
*   **`domain_id`** (int32): the domain to search in.
*   **`sender_ids`** (UUID array), **`types`** (int array): optional narrowing.
*   **`fields`** (string array): same projection as the history endpoints. Empty
    returns the default message payload.
*   **`cursor`** / **`size`**: keyset pagination, identical to
    `SearchThreadMessagesHistory` — `size` up to 100, `cursor.before` walks
    towards newer matches.

## Visibility

The result is limited to messages posted in a thread the caller joined, and
only within the periods they were a member of it:

```
thread_dialog.created_at <= message.created_at <= coalesce(thread_dialog.deleted_at, ∞)
```

So an operator sees the parts of the conversation they took part in — including
chats they have since left — and a client sees only their own dialogs. Deleted
messages are excluded; their text stays reachable through `GetMessageRevisions`
for members of the thread.

## Response

`SearchMessageHistoryResponse` — the same shape the history endpoints return:
`items` (each with `thread_id`), `from` (thread members for sender enrichment)
and `next_cursor` / `prev_cursor`.

## Jumping to a match

A hit is `(thread_id, id)`. To open the conversation around it, call
`SearchThreadMessagesHistory` with `thread_id` and the hit's id as the cursor,
once per direction.

Mind the direction flag — it reads inverted:

| cursor | returns |
|---|---|
| `{id, before: false}` | messages **older** than the hit |
| `{id, before: true}` | messages **newer** than the hit |

`before` maps onto the keyset direction, and the history is ordered by id
descending, so `before: true` walks towards the present. Asking for
`before: true` on the newest message of a thread correctly returns nothing.

## Indexing

`idx_messages_body_trgm` — a `gin_trgm_ops` index on `im_message.messages.body`
over live, non-empty bodies — backs the `ilike` scan; `pg_trgm` needs at least
three characters in the term to use it, shorter terms fall back to a scan of
the domain's messages.
