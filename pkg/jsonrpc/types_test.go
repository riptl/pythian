package jsonrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBatch(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		isBatch bool
	}{
		{
			name:    "single",
			data:    `{"jsonrpc":"2.0","method":"hello"}`,
			isBatch: false,
		},
		{
			name:    "batch",
			data:    `[{"jsonrpc":"2.0","method":"hello"},{"jsonrpc":"2.0","method":"hello"}]`,
			isBatch: true,
		},
		{
			name:    "invalid JSON",
			data:    `???`,
			isBatch: false,
		},
		{
			name:    "token",
			data:    `123`,
			isBatch: false,
		},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.isBatch, IsBatch([]byte(tc.data)))
	}
}
