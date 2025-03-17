package logutils

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNoop(t *testing.T) {
	logger := NewNoop()

	require.NotNil(t, logger)
	assert.IsType(t, (*slog.Logger)(nil), logger)
	assert.NotPanics(t, func() {
		logger.Info("foobar")
	})
}
