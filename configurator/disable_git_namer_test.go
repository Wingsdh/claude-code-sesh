package configurator

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/pathwrap"
	"github.com/Wingsdh/cc-sesh/v2/runtimewrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfig_DisableGitNamer(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected bool
	}{
		{"off when unset", "dir_length = 1\n", false},
		{"on when set to true", "disable_git_namer = true\n", true},
		{"accepted in strict mode", "strict_mode = true\ndisable_git_namer = true\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOs := &testOs{
				homeDir: "/home/testuser",
				files:   map[string][]byte{"/main/sesh.toml": []byte(tt.toml)},
			}

			c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, "/main/sesh.toml")
			config, err := c.GetConfig()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, config.DisableGitNamer)
		})
	}
}
