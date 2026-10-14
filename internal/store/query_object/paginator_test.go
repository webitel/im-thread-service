package queryobject

import (
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seqCursor struct {
	Seq int `json:"seq"`
	ID  int `json:"id"`
}

var seqColumns = []Column{
	{Name: "seq", Order: OrderDesc},
	{Name: "id", Order: OrderDesc},
}

var seqMapper = MapCursorMapper[seqCursor]{
	ToValuesFn: func(c seqCursor) (CursorValues, error) {
		return CursorValues{"seq": c.Seq, "id": c.ID}, nil
	},
	FromValuesFn: func(v CursorValues) (seqCursor, error) {
		return seqCursor{Seq: v["seq"].(int), ID: v["id"].(int)}, nil
	},
}

func seqConfig(limit uint64, dir Direction, cursor *seqCursor) Config[seqCursor] {
	cfg := Config[seqCursor]{
		Limit:     limit,
		Direction: dir,
		Columns:   seqColumns,
		Codec:     NewJSONBase64Codec[seqCursor](),
		Mapper:    seqMapper,
	}

	if cursor != nil {
		cfg.Cursor = *cursor
		cfg.HasCursor = true
	}

	return cfg
}

func seqBuilder() sq.SelectBuilder {
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Select("seq", "id").From("t").Where(sq.Eq{"k": 7})
}

func TestAroundLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		limit    uint64
		wantNext uint64
		wantPrev uint64
	}{
		{limit: 1, wantNext: 1, wantPrev: 0},
		{limit: 2, wantNext: 1, wantPrev: 1},
		{limit: 10, wantNext: 5, wantPrev: 5},
		{limit: 11, wantNext: 6, wantPrev: 5},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit %d", tt.limit), func(t *testing.T) {
			t.Parallel()

			next, prev := aroundLimits(tt.limit)

			assert.Equal(t, tt.wantNext, next)
			assert.Equal(t, tt.wantPrev, prev)
		})
	}
}

func TestSquirrelPaginator_ApplyAround_RequiresCursor(t *testing.T) {
	t.Parallel()

	_, _, err := New[seqCursor]().ApplyAround(seqBuilder(), seqConfig(10, DirectionAround, nil))

	require.Error(t, err)
}

func TestSquirrelPaginator_ApplyAround_MultiColumn(t *testing.T) {
	t.Parallel()

	sql, args, err := New[seqCursor]().ApplyAround(seqBuilder(), seqConfig(4, DirectionAround, &seqCursor{Seq: 3, ID: 9}))
	require.NoError(t, err)

	assert.Equal(t, normalizeSQL(
		"(SELECT seq, id FROM t WHERE k = $1 AND ((seq < $2) OR (seq = $3 AND id <= $4)) ORDER BY seq DESC, id DESC LIMIT 3)"+
			" UNION ALL "+
			"(SELECT seq, id FROM t WHERE k = $5 AND ((seq > $6) OR (seq = $7 AND id > $8)) ORDER BY seq ASC, id ASC LIMIT 3)"+
			" ORDER BY seq DESC, id DESC",
	), normalizeSQL(sql))
	assert.Equal(t, []any{7, 3, 3, 9, 7, 3, 3, 9}, args)
}

func TestBuildCursorPredicate_StaysStrict(t *testing.T) {
	t.Parallel()

	values := CursorValues{"seq": 3, "id": 9}

	tests := []struct {
		dir  Direction
		want string
	}{
		{dir: DirectionAfter, want: "((seq < ?) OR (seq = ? AND id < ?))"},
		{dir: DirectionBefore, want: "((seq > ?) OR (seq = ? AND id > ?))"},
	}

	for _, tt := range tests {
		t.Run(string(tt.dir), func(t *testing.T) {
			t.Parallel()

			pred, err := buildCursorPredicate(seqColumns, values, tt.dir)
			require.NoError(t, err)

			sql, _, err := pred.ToSql()
			require.NoError(t, err)
			assert.Equal(t, tt.want, sql)
		})
	}
}

func TestBuildPageInfo_RejectsAround(t *testing.T) {
	t.Parallel()

	rows := []int{3, 2, 1}

	_, err := BuildPageInfo(&rows, seqConfig(2, DirectionAround, &seqCursor{Seq: 2}), func(r int) (seqCursor, error) {
		return seqCursor{Seq: r}, nil
	})

	require.Error(t, err)
}

func TestBuildAroundPageInfo(t *testing.T) {
	t.Parallel()

	const anchor = 50

	tests := []struct {
		name     string
		limit    uint64
		rows     []int
		wantRows []int
		wantNext int
		wantPrev int
	}{
		{
			name:     "both sides overflow",
			limit:    10,
			rows:     []int{56, 55, 54, 53, 52, 51, 50, 49, 48, 47, 46, 45},
			wantRows: []int{55, 54, 53, 52, 51, 50, 49, 48, 47, 46},
			wantNext: 46,
			wantPrev: 55,
		},
		{
			name:     "anchor is the newest row",
			limit:    10,
			rows:     []int{50, 49, 48, 47, 46, 45},
			wantRows: []int{50, 49, 48, 47, 46},
			wantNext: 46,
		},
		{
			name:     "short on both sides",
			limit:    10,
			rows:     []int{52, 51, 50, 49},
			wantRows: []int{52, 51, 50, 49},
		},
		{
			name:     "anchor filtered out",
			limit:    4,
			rows:     []int{53, 52, 51, 49, 48},
			wantRows: []int{52, 51, 49, 48},
			wantPrev: 52,
		},
		{
			name:     "limit one keeps the anchor and points prev at it",
			limit:    1,
			rows:     []int{51, 50, 49},
			wantRows: []int{50},
			wantNext: 50,
			wantPrev: 50,
		},
		{
			name:     "empty window falls back to the anchor",
			limit:    1,
			rows:     []int{51},
			wantRows: []int{},
			wantPrev: anchor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := seqConfig(tt.limit, DirectionAround, &seqCursor{Seq: anchor})
			rows := append([]int(nil), tt.rows...)

			info, err := BuildAroundPageInfo(&rows, cfg,
				func(r int) (seqCursor, error) { return seqCursor{Seq: r}, nil },
				func(r int) bool { return r > anchor },
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantRows, rows)
			assert.Equal(t, tt.wantNext != 0, info.HasNextPage)
			assert.Equal(t, tt.wantPrev != 0, info.HasPrevPage)

			if tt.wantNext != 0 {
				token, err := cfg.Codec.Encode(seqCursor{Seq: tt.wantNext})
				require.NoError(t, err)

				assert.Equal(t, seqCursor{Seq: tt.wantNext}, info.NextCursor)
				assert.Equal(t, token, info.NextToken)
			}

			if tt.wantPrev != 0 {
				token, err := cfg.Codec.Encode(seqCursor{Seq: tt.wantPrev})
				require.NoError(t, err)

				assert.Equal(t, seqCursor{Seq: tt.wantPrev}, info.PrevCursor)
				assert.Equal(t, token, info.PrevToken)
			}
		})
	}
}
