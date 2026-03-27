# Updated Documentation: Cursor-Based Message History Pagination

This document reflects the latest API contract changes, specifically regarding the separation of Request and Response cursor objects and the movement of the `before` flag into the cursor request object.

---

## 1. Data Contracts

Pagination is managed via structured cursor objects in both the request and response.

### Request Parameters (`SearchMessageHistoryRequest`)

| Field | Type | Description |
| :--- | :--- | :--- |
| `size` | `uint32` | Number of messages per page. Maximum: **100**. |
| `cursor` | `HistoryMessageCursorRequest` | **Optional.** Contains the starting point and direction. |
| `thread_id` | `string` | The UUID of the thread to search within. |

**`HistoryMessageCursorRequest` Details:**
* `id` (string): The UUID of the message to start from.
* `before` (bool): 
    * `false` (default): Move **Forward** (towards older messages).
    * `true`: Move **Backward** (towards newer messages).

### Response Parameters (`SearchMessageHistoryResponse`)

| Field | Type | Description |
| :--- | :--- | :--- |
| `items` | `Array` | The list of messages. **Always sorted chronologically (oldest to newest).** |
| `next_cursor` | `HistoryMessageCursorResponse` | Cursor for the **next** page (older messages). Absent if the end of history is reached. |
| `prev_cursor` | `HistoryMessageCursorResponse` | Cursor for the **previous** page (newer messages). Absent if no newer messages exist. |

---

## 2. Usage Scenarios & Examples

### Scenario A: Initialization (First Request)
To get the most recent messages, send the request without a cursor.

**Request (JSON):**
```json
{
  "thread_id": "8489f635-...",
  "size": 20
}
```

**Response (JSON):**
```json
{
  "items": [
    { "id": "msg-001", "body": "Hello" },
    { "id": "msg-002", "body": "World" }
  ],
  "next_cursor": { "id": "msg-001" }
}
```

---

### Scenario B: Moving FORWARD (Load Older Messages)
To load the next (older) page, use the `next_cursor.id` from the previous response and set `before: false`.

**Request (JSON):**
```json
{
  "thread_id": "8489f635-...",
  "size": 20,
  "cursor": {
    "id": "msg-001",
    "before": false
  }
}
```

---

### Scenario C: Moving BACKWARD (Load Newer Messages)
To load newer messages (e.g., when scrolling down after being deep in history), use `prev_cursor.id` and set `before: true`.

**Request (JSON):**
```json
{
  "thread_id": "8489f635-...",
  "size": 20,
  "cursor": {
    "id": "msg-002",
    "before": true
  }
}
```

---

## 3. Implementation Details for Clients

1.  **Chronological Ordering:**
    The server automatically handles result sorting. Regardless of the `before` flag, the `items` array is returned in chronological order. The client should simply append (for `next_cursor`) or prepend (for `prev_cursor`) the items to the UI list.
2.  **Validation:**
    The `cursor.id` must be a valid UUID. If an invalid string is passed, the request will fail validation (Status 400).
3.  **Termination:**
    *   If `next_cursor` is missing: **End of history reached** (no older messages).
    *   If `prev_cursor` is missing: **Top of current view reached** (no newer messages).
4.  **Size Limits:**
    Requests with `size > 100` are automatically capped at 100.
