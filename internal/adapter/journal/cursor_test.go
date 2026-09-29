package journal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCursor(t *testing.T) {
	after, since, err := parseCursor("92547098")
	require.NoError(t, err)
	assert.Equal(t, int64(92547098), after)
	assert.True(t, since.IsZero(), "an API cursor is exact")

	after, since, err = parseCursor(LiveCursor(99, 1_790_000_000_000))
	require.NoError(t, err)
	assert.Equal(t, int64(99), after)
	assert.Equal(t, time.UnixMilli(1_790_000_000_000).Add(-LiveOverlap), since, "a live cursor replays the overlap")

	for _, bad := range []string{"abc", "1.x", "x.1", "."} {
		_, _, err := parseCursor(bad)
		require.ErrorIs(t, err, ErrInvalidCursor, bad)
	}
}
