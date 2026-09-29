package journal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCursor(t *testing.T) {
	after, err := parseCursor("92547098")
	require.NoError(t, err)
	assert.Equal(t, int64(92547098), after)

	for _, bad := range []string{"abc", "1.2", ""} {
		_, err := parseCursor(bad)
		require.ErrorIs(t, err, ErrInvalidCursor, bad)
	}
}
