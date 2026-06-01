package queryobject

import (
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

const (
	dialogsMsgAlias       string = "m"
	dialogsMsgDialogAlias string = "td"
)

const (
	dialogsMsgLinkThreadDialog = 1 << iota
)

// Keyset column qualified with the messages alias so ORDER BY / cursor
// predicate aren't ambiguous against the joined thread_dialog (which also
// exposes an `id` column).
var dialogsMessageHistoryColumns = []Column{
	{Name: dialogsMsgAlias + ".id", Order: OrderDesc},
}

// dialogsMessageHistoryCursorMapper mirrors MessageHistoryCursorMapper but
// keys the cursor values map by the qualified column name, since the paginator
// uses cols[i].Name both as the SQL reference and the map key.
type dialogsMessageHistoryCursorMapper struct{}

func (dialogsMessageHistoryCursorMapper) ToValues(c MessageHistoryCursor) (CursorValues, error) {
	if c.ID == uuid.Nil {
		return nil, fmt.Errorf("dialogsMessageHistoryCursorMapper: ID must not be nil UUID")
	}
	return CursorValues{dialogsMsgAlias + ".id": c.ID}, nil
}

func (dialogsMessageHistoryCursorMapper) FromValues(v CursorValues) (MessageHistoryCursor, error) {
	idRaw, ok := v[dialogsMsgAlias+".id"]
	if !ok {
		return MessageHistoryCursor{}, fmt.Errorf("dialogsMessageHistoryCursorMapper: missing %s.id", dialogsMsgAlias)
	}
	switch raw := idRaw.(type) {
	case uuid.UUID:
		return MessageHistoryCursor{ID: raw}, nil
	case string:
		id, err := uuid.Parse(raw)
		if err != nil {
			return MessageHistoryCursor{}, fmt.Errorf("dialogsMessageHistoryCursorMapper: invalid id %q: %w", raw, err)
		}
		return MessageHistoryCursor{ID: id}, nil
	default:
		return MessageHistoryCursor{}, fmt.Errorf("dialogsMessageHistoryCursorMapper: unsupported id type %T", idRaw)
	}
}

type dialogsMessageHistoryQueryObject struct {
	*baseQueryObject[*dialogsMessageHistoryQueryObject]

	paginatorCfg Config[MessageHistoryCursor]
	pag          *SquirrelPaginator[MessageHistoryCursor]
}

func NewDialogsMessageHistoryQueryObject() *dialogsMessageHistoryQueryObject {
	from := fmt.Sprintf("im_message.messages %s", dialogsMsgAlias)

	q := new(dialogsMessageHistoryQueryObject)
	q.baseQueryObject = newBaseQueryObject(from, q)
	q.pag = New[MessageHistoryCursor]()

	// Closed-history is meaningful only for messages whose dialog membership is deleted.
	q.EnsureJoins(dialogsMsgLinkThreadDialog)

	return q
}

func (q *dialogsMessageHistoryQueryObject) DefaultFields() []string {
	return []string{
		"id", "thread_id", "sender_id", "type", "body", "metadata",
		"created_at", "updated_at", "domain_id",
		"documents", "images", "location", "contact", "system",
		"interactive", "member",
	}
}

func (q *dialogsMessageHistoryQueryObject) FieldsMetadata() map[string]fieldMetadata {
	return map[string]fieldMetadata{
		"id":          {sqlExpr: "m.id", aliasedExpr: "m.id as id", sortable: true, filterExpr: "m.id"},
		"thread_id":   {sqlExpr: "m.thread_id", aliasedExpr: "m.thread_id as thread_id", filterExpr: "m.thread_id"},
		"sender_id":   {sqlExpr: "m.sender_id", aliasedExpr: "m.sender_id as sender_id", filterExpr: "m.sender_id"},
		"type":        {sqlExpr: "m.type", aliasedExpr: "m.type as type", filterExpr: "m.type"},
		"body":        {sqlExpr: "m.body", aliasedExpr: "m.body as body"},
		"metadata":    {sqlExpr: "m.metadata", aliasedExpr: "m.metadata as metadata"},
		"created_at":  {sqlExpr: "m.created_at", aliasedExpr: "m.created_at as created_at", sortable: true, filterExpr: "m.created_at"},
		"updated_at":  {sqlExpr: "m.updated_at", aliasedExpr: "m.updated_at as updated_at", sortable: true, filterExpr: "m.updated_at"},
		"domain_id":   {sqlExpr: "m.domain_id", aliasedExpr: "m.domain_id as domain_id", filterExpr: "m.domain_id"},
		"interactive": {sqlExpr: "m.interactive", aliasedExpr: "m.interactive as interactive"},

		"documents": {aliasedExpr: `(
			select jsonb_agg(jsonb_build_object(
				'id', md.id, 'file_id', md.file_id, 'name', md.name,
				'mime', md.mime, 'size', md.size, 'created_at', md.created_at
			))
			from im_message.message_documents md
			where md.message_id = m.id
			  and (m.type = 2 or (m.type = 5 and m.interactive->'attachments'->'documents' is not null))
		) as documents`},

		"images": {aliasedExpr: `(
			select jsonb_agg(jsonb_build_object(
				'id', mi.id, 'file_id', mi.file_id, 'mime', mi.mime,
				'thumbnails', mi.thumbnails, 'width', mi.width,
				'height', mi.height, 'created_at', mi.created_at
			))
			from im_message.message_images mi
			where mi.message_id = m.id
			  and (m.type = 3 or (m.type = 5 and m.interactive->'attachments'->'images' is not null))
		) as images`},

		"location": {aliasedExpr: `(
			select jsonb_build_object(
				'address', ml.address, 'name', ml.name,
				'latitude', ml.latitude, 'longitude', ml.longitude
			)
			from im_message.message_locations ml
			where m.type = 6 and ml.message_id = m.id
			limit 1
		) as location`},

		"contact": {aliasedExpr: `(
			select jsonb_build_object(
				'name', mc.name, 'phone_number', mc.phone_number, 'email', mc.email
			)
			from im_message.message_contacts mc
			where m.type = 7 and mc.message_id = m.id
			limit 1
		) as contact`},

		"system": {aliasedExpr: `(
			select jsonb_build_object('type', sm.type, 'metadata', sm.metadata)
			from im_message.system_messages sm
			where m.type = 4 and sm.message_id = m.id
			limit 1
		) as system`},

		"member": {
			// Keys match ThreadDialog's json tags so the jsonb decodes into *model.ThreadDialog.
			aliasedExpr: fmt.Sprintf(`jsonb_build_object(
				'id', %[1]s.id,
				'member_id', %[1]s.member_id,
				'thread_id', %[1]s.thread_id,
				'member_role', %[1]s.thread_role,
				'invited_by', %[1]s.invited_by,
				'leave_reason', %[1]s.leave_reason,
				'deleted_at', %[1]s.deleted_at,
				'created_at', %[1]s.created_at,
				'updated_at', %[1]s.updated_at
			) as member`, dialogsMsgDialogAlias),
			requiresJoin: dialogsMsgLinkThreadDialog,
		},
	}
}

func (q *dialogsMessageHistoryQueryObject) EnsureJoins(requiredJoin int) {
	if requiredJoin&dialogsMsgLinkThreadDialog != 0 {
		q.linkThreadDialog()
	}
}

func (q *dialogsMessageHistoryQueryObject) linkThreadDialog() {
	if q.join&dialogsMsgLinkThreadDialog != 0 {
		return
	}
	q.join |= dialogsMsgLinkThreadDialog
	q.builder = q.builder.InnerJoin(fmt.Sprintf(
		"%s %s on %s.id = %s.member_id and %s.deleted_at is not null",
		ThreadDialogTable, dialogsMsgDialogAlias,
		dialogsMsgDialogAlias, dialogsMsgAlias,
		dialogsMsgDialogAlias,
	))
}

func (q *dialogsMessageHistoryQueryObject) WithDomainIDFilter(domainID int) *dialogsMessageHistoryQueryObject {
	if domainID > 0 {
		q.builder = q.builder.Where(squirrel.Eq{dialogsMsgAlias + ".domain_id": domainID})
	}
	return q
}

func (q *dialogsMessageHistoryQueryObject) WithThreadIDFilter(threadIDs ...uuid.UUID) *dialogsMessageHistoryQueryObject {
	if len(threadIDs) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{dialogsMsgAlias + ".thread_id": threadIDs})
	}
	return q
}

func (q *dialogsMessageHistoryQueryObject) WithSenderIDsFilter(senderIDs ...uuid.UUID) *dialogsMessageHistoryQueryObject {
	if len(senderIDs) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{dialogsMsgAlias + ".sender_id": senderIDs})
	}
	return q
}

func (q *dialogsMessageHistoryQueryObject) WithTypesFilter(types ...int) *dialogsMessageHistoryQueryObject {
	if len(types) != 0 {
		q.builder = q.builder.Where(squirrel.Eq{dialogsMsgAlias + ".type": types})
	}
	return q
}

// WithPeriodFilter narrows results to dialog-membership windows that overlap the
// given period: deleted_at >= periodFrom and created_at <= periodTo. A zero
// time.Time on either side leaves that bound open.
func (q *dialogsMessageHistoryQueryObject) WithPeriodFilter(periodFrom, periodTo time.Time) *dialogsMessageHistoryQueryObject {
	if !periodFrom.IsZero() {
		q.builder = q.builder.Where(
			fmt.Sprintf("%s.deleted_at >= ?", dialogsMsgDialogAlias),
			periodFrom,
		)
	}
	if !periodTo.IsZero() {
		q.builder = q.builder.Where(
			fmt.Sprintf("%s.created_at <= ?", dialogsMsgDialogAlias),
			periodTo,
		)
	}
	return q
}

func (q *dialogsMessageHistoryQueryObject) WithCursor(cursor *dto.HistoryMessageCursor) *dialogsMessageHistoryQueryObject {
	if cursor == nil {
		return q
	}
	cfg, err := NewMessageHistoryConfigFromRaw(
		q.pageLimitOrDefault(),
		MessageHistoryCursor{ID: cursor.Id},
		cursor.Direction,
	)
	if err != nil {
		return q
	}
	cfg.Columns = dialogsMessageHistoryColumns
	cfg.Mapper = dialogsMessageHistoryCursorMapper{}
	q.paginatorCfg = cfg
	return q
}

// WithLimit shadows baseQueryObject.WithLimit because pagination is owned by the
// cursor paginator, not the squirrel builder.
func (q *dialogsMessageHistoryQueryObject) WithLimit(limit int) *dialogsMessageHistoryQueryObject {
	if limit > 0 && limit <= 100 {
		q.paginatorCfg.Limit = uint64(limit)
	}
	return q
}

func (q *dialogsMessageHistoryQueryObject) ToSql() (string, []any, error) {
	if q.paginatorCfg.Limit == 0 {
		q.paginatorCfg.Limit = uint64(DefaultLimit)
	}
	if q.paginatorCfg.Codec == nil {
		q.paginatorCfg.Codec = NewJSONBase64Codec[MessageHistoryCursor]()
		q.paginatorCfg.Mapper = dialogsMessageHistoryCursorMapper{}
		q.paginatorCfg.Columns = dialogsMessageHistoryColumns
		q.paginatorCfg.Direction = DirectionAfter
	}

	if len(q.fields) < 1 {
		q.fields = q.DefaultFields()
	}

	meta := q.FieldsMetadata()
	selectExprs := make([]string, 0, len(q.fields))
	for _, f := range q.fields {
		m, ok := meta[f]
		if !ok {
			continue
		}
		q.EnsureJoins(m.requiresJoin)
		selectExprs = append(selectExprs, m.aliasedExpr)
	}

	builder := q.builder.Columns(selectExprs...)
	decorated, err := q.pag.Apply(builder, q.paginatorCfg)
	if err != nil {
		return "", nil, err
	}

	sql, args, err := decorated.ToSql()
	if err != nil {
		return "", nil, err
	}
	return CompactSQL(sql), args, nil
}

func (q *dialogsMessageHistoryQueryObject) BuildPageInfo(
	rows *[]*model.Message,
	extract CursorExtractor[*model.Message, MessageHistoryCursor],
) (PageInfo[MessageHistoryCursor], error) {
	return BuildPageInfo(rows, q.paginatorCfg, extract)
}

func (q *dialogsMessageHistoryQueryObject) pageLimitOrDefault() uint64 {
	if q.paginatorCfg.Limit > 0 {
		return q.paginatorCfg.Limit
	}
	return uint64(DefaultLimit)
}
