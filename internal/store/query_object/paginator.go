package queryobject

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
)

type Direction string

const (
	DirectionAfter  Direction = "after"
	DirectionBefore Direction = "before"
)

type Order string

const (
	OrderAsc  Order = "ASC"
	OrderDesc Order = "DESC"
)

func (o Order) reverse() Order {
	if o == OrderAsc {
		return OrderDesc
	}
	return OrderAsc
}

type Cursor string

type CursorValues map[string]any

type Column struct {
	Name  string
	Order Order
}

type CursorCodec[C any] interface {
	Encode(C) (Cursor, error)
	Decode(Cursor) (C, bool, error)
}

type CursorMapper[C any] interface {
	ToValues(C) (CursorValues, error)
	FromValues(CursorValues) (C, error)
}

type JSONBase64Codec[C any] struct{}

func NewJSONBase64Codec[C any]() JSONBase64Codec[C] {
	return JSONBase64Codec[C]{}
}

func (JSONBase64Codec[C]) Encode(c C) (Cursor, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("paginator: JSONBase64Codec encode: %w", err)
	}
	return Cursor(base64.StdEncoding.EncodeToString(b)), nil
}

func (JSONBase64Codec[C]) Decode(cur Cursor) (C, bool, error) {
	var zero C
	if cur == "" {
		return zero, false, nil
	}
	b, err := base64.StdEncoding.DecodeString(string(cur))
	if err != nil {
		return zero, false, fmt.Errorf("paginator: JSONBase64Codec decode base64: %w", err)
	}
	var c C
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		return zero, false, fmt.Errorf("paginator: JSONBase64Codec decode json: %w", err)
	}
	return c, true, nil
}

type MapCursorMapper[C any] struct {
	ToValuesFn   func(C) (CursorValues, error)
	FromValuesFn func(CursorValues) (C, error)
}

func (m MapCursorMapper[C]) ToValues(c C) (CursorValues, error) {
	return m.ToValuesFn(c)
}
func (m MapCursorMapper[C]) FromValues(v CursorValues) (C, error) {
	return m.FromValuesFn(v)
}

type Config[C any] struct {
	Limit     uint64
	Cursor    C
	HasCursor bool
	Direction Direction
	Columns   []Column
	Codec     CursorCodec[C]
	Mapper    CursorMapper[C]
}

type PageInfo[C any] struct {
	HasNextPage bool
	HasPrevPage bool
	NextCursor  C
	PrevCursor  C
	NextToken   Cursor
	PrevToken   Cursor
}

type CursorExtractor[Row any, C any] func(Row) (C, error)

type Paginator[C any] interface {
	Apply(builder sq.SelectBuilder, cfg Config[C]) (sq.SelectBuilder, error)
}

type SquirrelPaginator[C any] struct{}

func New[C any]() *SquirrelPaginator[C] {
	return &SquirrelPaginator[C]{}
}

func (p *SquirrelPaginator[C]) Apply(builder sq.SelectBuilder, cfg Config[C]) (sq.SelectBuilder, error) {
	if err := validateConfig(cfg); err != nil {
		return builder, err
	}

	var cursorValues CursorValues
	if cfg.HasCursor {
		cv, err := cfg.Mapper.ToValues(cfg.Cursor)
		if err != nil {
			return builder, fmt.Errorf("paginator: cursor mapper ToValues: %w", err)
		}
		cursorValues = cv
	}

	builder = applyOrderBy(builder, cfg.Columns, cfg.Direction)

	if cursorValues != nil {
		pred, err := buildCursorPredicate(cfg.Columns, cursorValues, cfg.Direction)
		if err != nil {
			return builder, err
		}
		builder = builder.Where(pred)
	}

	builder = builder.Limit(cfg.Limit + 1)

	return builder, nil
}

func BuildPageInfo[Row any, C any](
	rows *[]Row,
	cfg Config[C],
	extract CursorExtractor[Row, C],
) (PageInfo[C], error) {
	if err := validateConfig(cfg); err != nil {
		return PageInfo[C]{}, err
	}

	limit := int(cfg.Limit)
	hasExtra := len(*rows) > limit

	if hasExtra {
		*rows = (*rows)[:limit]
	}

	if cfg.Direction == DirectionBefore {
		reverseSlice(*rows)
	}

	if len(*rows) == 0 {
		return PageInfo[C]{}, nil
	}

	var info PageInfo[C]

	switch cfg.Direction {
	case DirectionAfter:
		info.HasPrevPage = cfg.HasCursor
		info.HasNextPage = hasExtra
	case DirectionBefore:
		info.HasNextPage = cfg.HasCursor
		info.HasPrevPage = hasExtra
	}

	if info.HasNextPage {
		last := (*rows)[len(*rows)-1]
		cur, err := extract(last)
		if err != nil {
			return PageInfo[C]{}, fmt.Errorf("paginator: extract next cursor: %w", err)
		}
		token, err := cfg.Codec.Encode(cur)
		if err != nil {
			return PageInfo[C]{}, fmt.Errorf("paginator: encode next cursor: %w", err)
		}
		info.NextCursor = cur
		info.NextToken = token
	}

	if info.HasPrevPage {
		first := (*rows)[0]
		cur, err := extract(first)
		if err != nil {
			return PageInfo[C]{}, fmt.Errorf("paginator: extract prev cursor: %w", err)
		}
		token, err := cfg.Codec.Encode(cur)
		if err != nil {
			return PageInfo[C]{}, fmt.Errorf("paginator: encode prev cursor: %w", err)
		}
		info.PrevCursor = cur
		info.PrevToken = token
	}

	return info, nil
}

func validateConfig[C any](cfg Config[C]) error {
	if cfg.Limit == 0 {
		return fmt.Errorf("paginator: Limit must be > 0")
	}

	if len(cfg.Columns) == 0 {
		return fmt.Errorf("paginator: at least one Column is required")
	}

	for i, c := range cfg.Columns {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("paginator: column[%d] has empty Name", i)
		}
		if c.Order != OrderAsc && c.Order != OrderDesc {
			return fmt.Errorf("paginator: column[%d] has invalid Order %q", i, c.Order)
		}
	}

	if cfg.Direction != DirectionAfter && cfg.Direction != DirectionBefore && cfg.Direction != "" {
		return fmt.Errorf("paginator: invalid Direction %q", cfg.Direction)
	}

	if cfg.Codec == nil {
		return fmt.Errorf("paginator: Codec is required")
	}

	if cfg.Mapper == nil {
		return fmt.Errorf("paginator: Mapper is required")
	}

	return nil
}

func effectiveOrder(col Column, dir Direction) Order {
	if dir == DirectionBefore {
		return col.Order.reverse()
	}
	return col.Order
}

func applyOrderBy(builder sq.SelectBuilder, cols []Column, dir Direction) sq.SelectBuilder {
	for _, col := range cols {
		builder = builder.OrderBy(fmt.Sprintf("%s %s", col.Name, effectiveOrder(col, dir)))
	}
	return builder
}

func buildCursorPredicate(cols []Column, values CursorValues, dir Direction) (sq.Sqlizer, error) {
	for _, col := range cols {
		if _, ok := values[col.Name]; !ok {
			return nil, fmt.Errorf("paginator: cursor is missing value for column %q", col.Name)
		}
	}

	var or sq.Or
	for i := range cols {
		var and sq.And
		for j := range i {
			and = append(and, sq.Eq{cols[j].Name: normaliseJSONNumber(values[cols[j].Name])})
		}
		op := inequalityOp(cols[i].Order, dir)
		and = append(and, sq.Expr(
			fmt.Sprintf("%s %s ?", cols[i].Name, op),
			normaliseJSONNumber(values[cols[i].Name]),
		))
		or = append(or, and)
	}
	return or, nil
}

func inequalityOp(order Order, dir Direction) string {
	if (order == OrderAsc && dir == DirectionAfter) ||
		(order == OrderDesc && dir == DirectionBefore) {
		return ">"
	}
	return "<"
}

func normaliseJSONNumber(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return v
}

func reverseSlice[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
