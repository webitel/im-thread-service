# Documentation: Cursor-Based Message History Pagination

This document outlines the implementation of cursor-based pagination for retrieving message history. This method uses unique message identifiers (cursors) and a direction flag to ensure stable pagination, even as new messages are being added to the database.

---

## 1. Data Contracts

Pagination is managed through specific fields in both the request and response objects.

### Request Parameters (`SearchMessageHistoryRequest`)

| Field | Type | Description |
| :--- | :--- | :--- |
| `size` | `uint32` | The number of messages requested per page (limit). Maximum value is **100**. If `0` or >`100` is provided, the server uses a default value. |
| `cursor` | `HistoryMessageCursor` | The cursor object containing the message `id` where the selection starts. Leave empty for the initial request (to get the latest messages). |
| `before` | `bool` | The direction of traversal. `false` (default) moves **Forward** (deeper into history/older messages). `true` moves **Backward** (towards newer messages). |

### Response Parameters (`SearchMessageHistoryResponse`)

| Field | Type | Description |
| :--- | :--- | :--- |
| `messages` | `Array` | An array of messages for the current page. **Always sorted in chronological order**, regardless of the request direction. |
| `next_cursor` | `HistoryMessageCursor` | The cursor used to fetch the **next** page (older messages). If missing, the end of the history has been reached. |
| `prev_cursor` | `HistoryMessageCursor` | The cursor used to fetch the **previous** page (newer messages). If missing, there are no newer messages available. |

---

## 2. Usage Scenarios & Examples

### Scenario A: Initialization (First Request)

When opening a chat, the client makes an initial request without a cursor to retrieve the most recent page of messages.

**Request (JSON):**
```json
{
  "thread_id": "123e4567-e89b-12d3-a456-426614174000",
  "size": 50,
  "before": false
}
```

**Response (JSON):**
```json
{
  "messages": [
    { "id": "msg-1", "body": "Hello", "created_at": "..." },
    { "id": "msg-2", "body": "How are you?", "created_at": "..." }
  ],
  "next_cursor": {
    "id": "msg-1"
  }
}
```
> **Note:** Since this is the most recent page, `prev_cursor` will be absent. Save `next_cursor.id` for subsequent history loading.

---

### Scenario B: Moving FORWARD (Scroll Up / Load Older Messages)

To load older messages, use the `next_cursor` from the previous response. Keep `before` set to `false`.

**Request (JSON):**
```json
{
  "thread_id": "123e4567-e89b-12d3-a456-426614174000",
  "size": 50,
  "before": false,
  "cursor": {
    "id": "msg-1"
  }
}
```

**Response (JSON):**
```json
{
  "messages": [
    { "id": "msg-10", "body": "Old message 1", "created_at": "..." },
    { "id": "msg-11", "body": "Old message 2", "created_at": "..." }
  ],
  "next_cursor": {
    "id": "msg-10"
  },
  "prev_cursor": {
    "id": "msg-11"
  }
}
```

---

### Scenario C: Moving BACKWARD (Scroll Down / Load Newer Messages)

This is used when a user has scrolled far up and begins scrolling back down toward newer messages. Provide the `prev_cursor` and set `before` to `true`.

**Request (JSON):**
```json
{
  "thread_id": "123e4567-e89b-12d3-a456-426614174000",
  "size": 50,
  "before": true,
  "cursor": {
    "id": "msg-11"
  }
}
```

---

## 3. Implementation Details for Clients

1.  **Chronological Ordering:**
    Even when requesting with `before: true`, the backend automatically reverses the result set before delivery. The client **does not need** to manually sort or reverse the `messages` array. Elements are always ready for top-to-bottom rendering (oldest to newest).
2.  **Termination Criteria:**
    * **End of history** (no older messages): `next_cursor` will be missing from the response.
    * **Start of history** (no newer messages): `prev_cursor` will be missing from the response.
    The client should stop making requests in the respective direction if the corresponding cursor is not returned.
3.  **Size Limits:**
    It is recommended to request between 20 and 50 messages (`size`). The backend enforces a hard limit: requests with `size > 100` are automatically capped to 100 to maintain database performance.