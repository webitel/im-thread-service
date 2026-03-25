package queryobject

import (
	"fmt"

	"github.com/google/uuid"
)

type MessageHistoryCursor struct {	
	ID        uuid.UUID `json:"id"`
	Before bool
}

var MessageHistoryColumns = []Column{
	{Name: "id", Order: OrderDesc},
}

type MessageHistoryCursorMapper struct{}

func (MessageHistoryCursorMapper) ToValues(c MessageHistoryCursor) (CursorValues, error) {
	if c.ID == uuid.Nil {
		return nil, fmt.Errorf("MessageHistoryCursorMapper: ID must not be nil UUID")
	}

	return CursorValues{
		"id":         c.ID,
	}, nil
}

func (MessageHistoryCursorMapper) FromValues(v CursorValues) (MessageHistoryCursor, error) {
	idRaw, ok := v["id"]
	if !ok {
		return MessageHistoryCursor{}, fmt.Errorf("MessageHistoryCursorMapper: missing id")
	}

	var id uuid.UUID
	switch raw := idRaw.(type) {
	case uuid.UUID:
		id = raw
	case string:
		var err error
		id, err = uuid.Parse(raw)
		if err != nil {
			return MessageHistoryCursor{}, fmt.Errorf("MessageHistoryCursorMapper: invalid id %q: %w", raw, err)
		}
	default:
		return MessageHistoryCursor{}, fmt.Errorf("MessageHistoryCursorMapper: unsupported id type %T", idRaw)
	}

	return MessageHistoryCursor{ID: id}, nil
}

type RawCursorParser[C any] func(raw Cursor) (C, bool, error)

type RawParamsCursorCodec[C any] struct {
	parser RawCursorParser[C]
}

func NewRawParamsCursorCodec[C any](parser RawCursorParser[C]) RawParamsCursorCodec[C] {
	return RawParamsCursorCodec[C]{parser: parser}
}

func (c RawParamsCursorCodec[C]) Encode(cur C) (Cursor, error) {
	return "", nil
}

func (c RawParamsCursorCodec[C]) Decode(raw Cursor) (C, bool, error) {
	return c.parser(raw)
}

func NewMessageHistoryConfig(
	limit uint64,
	rawToken Cursor,
	before bool,
) (Config[MessageHistoryCursor], error) {
	codec := NewJSONBase64Codec[MessageHistoryCursor]()

	cur, ok, err := codec.Decode(rawToken)
	if err != nil {
		return Config[MessageHistoryCursor]{}, fmt.Errorf("invalid cursor token: %w", err)
	}

	dir := DirectionAfter
	if before {
		dir = DirectionBefore
	}

	return Config[MessageHistoryCursor]{
		Limit:     limit,
		Cursor:    cur,
		HasCursor: ok,
		Direction: dir,
		Columns:   MessageHistoryColumns,
		Codec:     codec,
		Mapper:    MessageHistoryCursorMapper{},
	}, nil
}

func NewMessageHistoryConfigFromRaw(limit uint64, raw MessageHistoryCursor, before bool) (Config[MessageHistoryCursor], error) {
	codec := NewRawParamsCursorCodec(parseMessageHistoryRawCursor)
	cur, hasCursor, err := parseMessageHistoryFields(raw.ID)
	if err != nil {
		return Config[MessageHistoryCursor]{}, fmt.Errorf("invalid cursor params: %w", err)
	}

	dir := DirectionAfter
	if before {
		dir = DirectionBefore
	}

	return Config[MessageHistoryCursor]{
		Limit:     limit,
		Cursor:    cur,
		HasCursor: hasCursor,
		Direction: dir,
		Columns:   MessageHistoryColumns,
		Codec:     codec,
		Mapper:    MessageHistoryCursorMapper{},
	}, nil
}

func parseMessageHistoryRawCursor(raw Cursor) (MessageHistoryCursor, bool, error) {
	if raw == "" {
		return MessageHistoryCursor{}, false, nil
	}

	return NewJSONBase64Codec[MessageHistoryCursor]().Decode(raw)
}

func parseMessageHistoryFields(rawID uuid.UUID) (MessageHistoryCursor, bool, error) {
	if rawID == uuid.Nil {
		return MessageHistoryCursor{}, false, nil
	}	

	return MessageHistoryCursor{
		ID:        rawID,
	}, true, nil
}
