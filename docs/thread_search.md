# Thread Search Endpoint Documentation

This endpoint is designed to retrieve a list of dialogues (threads) with support for filtering, pagination, sorting, and selective field loading (including the last message).

## Request (ThreadSearchRequest)

### 1. Data Selection (Fields)
*   **`fields`** (string array): Defines which fields should be returned in the response.
    *   **Note:** If the array is empty, the backend returns a default set: `id`, `domain_id`, `created_at`, `updated_at`, `kind`, `owner`, `subject`, `description`, `member_ids`.
    *   To include the last message object, you must explicitly pass `"last_msg"` in the `fields` array.
    *   To retrieve detailed member information, pass `"members"`.

### 2. Pagination and Limits
*   **`page`** (int32): Page number. Starts at 1.
*   **`size`** (int32): Number of records per page. Minimum 1, maximum 100.
    *   *Under the hood:* The backend requests `size + 1` records. If more than `size` records are returned, the `next` flag in the response is set to `true`.

### 3. Sorting
*   **`sort`** (string): The field to sort by, with a mandatory direction prefix.
    *   `+` (Ascending / ASC)
    *   `-` (Descending / DESC)
*   **Sortable fields:** `id`, `domain_id`, `created_at`, `updated_at`, `kind`, `owner`, `subject`, `description`, `last_msg_at`.
*   **Example:** For a standard chat list ordered from newest to oldest, use `"-last_msg_at"` or `"-updated_at"`.

### 4. Filtering and Search
*   **`q`** (string): Full-text search (up to 256 characters). Searches for matches in the `subject` field (for groups/channels) or `title` (for direct chats) using case-insensitive matching (`ilike`).
*   **`ids`** (UUID array): Filter by specific thread IDs.
*   **`domain_ids`** (int32 array): Filter by domain (must be > 0).
*   **`kinds`** (enum array): Filter by thread type (1 - DIRECT, 2 - GROUP, 3 - CHANNEL). **0 (UNKNOWN) is prohibited.**
*   **`owners`** (UUID array): Filter by thread owner IDs.
*   **`member_ids`** (UUID array): Filter threads where the specified users are members.

---

## Response (SearchThreadResponse)

*   **`items`** (`Thread` object array): The found dialogues containing the fields requested in the `fields` parameter.
*   **`next`** (boolean): Indicates if a next page exists. Used by clients to implement infinite scrolling.

---

## Guidelines for Clients

1.  **Chat Naming (Subject):**
    Clients do not need to manually calculate names for Direct chats. If the `subject` field is requested, the backend automatically returns the `title` from Direct settings (if `kind == 1`), or the standard `subject` for groups and channels.
2.  **Retrieving the Last Message:**
    To display the last message preview in the chat list, ensure `"last_msg"` is added to `ThreadSearchRequest.fields`. The `last_msg` object includes the text, type, sender, metadata, and attachments (documents, images with thumbnails).
3.  **Traffic Optimization:**
    Avoid requesting `"members"` (full member data) in the general chat list unless necessary for the UI. Use the default field set or `"member_ids"` for lazy loading instead.
4.  **Chat List Ordering:**
    To correctly sort chats based on new activity, pass `sort: "-last_msg_at"`. This performs a JOIN with the messages table and sorts by the ID of the most recent message.
