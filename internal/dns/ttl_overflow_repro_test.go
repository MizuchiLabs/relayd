package dns

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTTLHugeSecondsClampsToMax(t *testing.T) {
	t.Parallel()
	got, err := ParseTTL("10000000000")
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, got)
}
