package queryobject

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type MessageHistoryCursor struct {
	ID     uuid.UUID `json:"id"`
	Before bool
}

var MessageHistoryColumns = []Column{
	{Name: "id", Order: OrderDesc},
}

type MessageHistoryCursorMapper struct{}

func (MessageHistoryCursorMapper) ToValues(c MessageHistoryCursor) (CursorValues, error) {
	if c.ID == uuid.Nil {
		return nil, errors.New("MessageHistoryCursorMapper: ID must not be nil UUID")
	}

	return CursorValues{
		"id": c.ID,
	}, nil
}

func (MessageHistoryCursorMapper) FromValues(v CursorValues) (MessageHistoryCursor, error) {
	idRaw, ok := v["id"]
	if !ok {
		return MessageHistoryCursor{}, errors.New("MessageHistoryCursorMapper: missing id")
	}

	var id uuid.UUID

	switch raw := idRaw.(type) {
	case uuid.UUID:
		id = raw
	case string:
		var err error

		id, err = uuid.Parse(raw)
		if err != nil {
			return MessageHistoryCursor{}, errors.InvalidArgument(
				fmt.Sprintf("MessageHistoryCursorMapper: invalid id %q", raw),
				errors.WithCause(err),
				errors.WithID("queryobject.MessageHistoryCursorMapper.FromValues"),
			)
		}
	default:
		return MessageHistoryCursor{}, errors.InvalidArgument(
			fmt.Sprintf("MessageHistoryCursorMapper: unsupported id type %T", idRaw),
			errors.WithID("queryobject.MessageHistoryCursorMapper.FromValues"),
		)
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

func (c RawParamsCursorCodec[C]) Encode(_ C) (Cursor, error) {
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
		return Config[MessageHistoryCursor]{}, errors.InvalidArgument(
			"invalid cursor token",
			errors.WithCause(err),
			errors.WithID("queryobject.NewMessageHistoryConfig"),
		)
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
		return Config[MessageHistoryCursor]{}, errors.InvalidArgument(
			"invalid cursor params",
			errors.WithCause(err),
			errors.WithID("queryobject.NewMessageHistoryConfigFromRaw"),
		)
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
		ID: rawID,
	}, true, nil
}
