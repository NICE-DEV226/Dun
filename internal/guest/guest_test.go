package guest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrapModule(t *testing.T) {
	module := TrapModule()
	require.NotEmpty(t, module)
	// Module valide selon la spec WAV (magic + version + sections).
	err := ValidateWasm(module)
	require.NoError(t, err, "TrapModule should produce valid wasm")
}

func TestInfiniteLoopModule(t *testing.T) {
	module := InfiniteLoopModule()
	require.NotEmpty(t, module)
	err := ValidateWasm(module)
	require.NoError(t, err, "InfiniteLoopModule should produce valid wasm")
}

func TestMemoryModule(t *testing.T) {
	tests := []struct {
		name     string
		minPages uint32
	}{
		{"1 page", 1},
		{"2 pages", 2},
		{"max uint32", ^uint32(0)},
		{"0 pages", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := MemoryModule(tt.minPages)
			require.NotEmpty(t, module)
			err := ValidateWasm(module)
			require.NoError(t, err, "MemoryModule(%d) should produce valid wasm", tt.minPages)
		})
	}
}

func TestSecurityHeaderModule(t *testing.T) {
	module := SecurityHeaderModule()
	require.NotEmpty(t, module)
	err := ValidateWasm(module)
	require.NoError(t, err, "SecurityHeaderModule should produce valid wasm")
}

func TestSecurityHeaderModuleValue(t *testing.T) {
	tests := []struct {
		name     string
		hdrName  string
		hdrValue string
	}{
		{"default strings", "X-Custom", "my-value"},
		{"boundary name", string(make([]byte, 31)), "value"},
		{"boundary value", "name", string(make([]byte, 31))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := SecurityHeaderModuleValue(tt.hdrName, tt.hdrValue)
			require.NotEmpty(t, module)
			err := ValidateWasm(module)
			require.NoError(t, err, "SecurityHeaderModuleValue(%s,%s) should produce valid wasm", tt.hdrName, tt.hdrValue)
		})
	}
}

func TestValidateWasm(t *testing.T) {
	tests := []struct {
		name    string
		b       []byte
		wantErr bool
	}{
		{"valid minimal", []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, false},
		{"too short", []byte{0x00, 0x61, 0x73, 0x6d}, true},
		{"wrong magic", []byte{0x01, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, true},
		{"wrong version", []byte{0x00, 0x61, 0x73, 0x6d, 0x02, 0x00, 0x00, 0x00}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWasm(tt.b)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
