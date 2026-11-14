package configurator

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/pathwrap"
	"github.com/Wingsdh/cc-sesh/v2/runtimewrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfig_NameSubstitutionRulesLoadInOrder(t *testing.T) {
	toml := []byte(`
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""

[[name_substitution]]
find = "nodemicro"
replace = "nm"
`)
	mockOs := &testOs{
		homeDir: "/home/testuser",
		files:   map[string][]byte{"/main/sesh.toml": toml},
	}

	c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, "/main/sesh.toml")
	config, err := c.GetConfig()

	require.NoError(t, err)
	assert.Equal(t, []model.NameSubstitution{
		{Find: "~/Workspace/worktrees/", Replace: ""},
		{Find: "nodemicro", Replace: "nm"},
	}, config.NameSubstitutions)
}

func TestGetConfig_NameSubstitutionWithEmptyFindFails(t *testing.T) {
	toml := []byte(`
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""

[[name_substitution]]
find = ""
replace = "x"
`)
	tests := []struct {
		name       string
		configPath string
		filePath   string
	}{
		{"custom config path", "/main/sesh.toml", "/main/sesh.toml"},
		{"default config dir", "", "/home/testuser/.config/cc-sesh/cc-sesh.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOs := &testOs{
				homeDir: "/home/testuser",
				files:   map[string][]byte{tt.filePath: toml},
			}

			c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, tt.configPath)
			_, err := c.GetConfig()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "name_substitution rule 2 has an empty find")
		})
	}
}
