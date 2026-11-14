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

func TestGetConfig_NameSubstitutionWithInvalidRegexFails(t *testing.T) {
	toml := []byte(`
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""

[[name_substitution]]
find = "(unterminated"
replace = "x"
regex = true
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
			assert.Contains(t, err.Error(), `invalid name_substitution regex "(unterminated"`)
			// The regexp compile error says what's wrong with the pattern.
			assert.Contains(t, err.Error(), "missing closing )")
		})
	}
}

func TestGetConfig_NameSubstitutionRegexIsOptIn(t *testing.T) {
	toml := []byte(`
[[name_substitution]]
find = "(unterminated"
replace = "x"

[[name_substitution]]
find = "^([0-9]+)_(.*)$"
replace = "$2-$1"
regex = true
`)
	mockOs := &testOs{
		homeDir: "/home/testuser",
		files:   map[string][]byte{"/main/sesh.toml": toml},
	}

	c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, "/main/sesh.toml")
	config, err := c.GetConfig()

	// Without regex = true the find is a literal, so it doesn't have to be a
	// valid pattern.
	require.NoError(t, err)
	assert.Equal(t, []model.NameSubstitution{
		{Find: "(unterminated", Replace: "x", Regex: false},
		{Find: "^([0-9]+)_(.*)$", Replace: "$2-$1", Regex: true},
	}, config.NameSubstitutions)
}

func TestGetConfig_NameSubstitutionRulesFromImportsFollowMainRules(t *testing.T) {
	mainTOML := []byte(`
import = ["~/imports/first.toml", "~/imports/second.toml"]

[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""
`)
	firstTOML := []byte(`
[[name_substitution]]
find = "nodemicro"
replace = "nm"
`)
	secondTOML := []byte(`
[[name_substitution]]
find = "^([0-9]+)_(.*)$"
replace = "$2-$1"
regex = true
`)
	mockOs := &testOs{
		homeDir: "/home/testuser",
		files: map[string][]byte{
			"/main/sesh.toml":                    mainTOML,
			"/home/testuser/imports/first.toml":  firstTOML,
			"/home/testuser/imports/second.toml": secondTOML,
		},
	}

	c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, "/main/sesh.toml")
	config, err := c.GetConfig()

	require.NoError(t, err)
	assert.Equal(t, []model.NameSubstitution{
		{Find: "~/Workspace/worktrees/", Replace: ""},
		{Find: "nodemicro", Replace: "nm"},
		{Find: "^([0-9]+)_(.*)$", Replace: "$2-$1", Regex: true},
	}, config.NameSubstitutions)
}

func TestGetConfig_InvalidNameSubstitutionInImportFails(t *testing.T) {
	rules := []struct {
		name       string
		importTOML string
		wantErr    string
	}{
		{
			name:       "empty find",
			importTOML: "[[name_substitution]]\nfind = \"\"\nreplace = \"x\"\n",
			wantErr:    "has an empty find",
		},
		{
			name:       "invalid regex",
			importTOML: "[[name_substitution]]\nfind = \"(unterminated\"\nreplace = \"x\"\nregex = true\n",
			wantErr:    `invalid name_substitution regex "(unterminated"`,
		},
	}
	configs := []struct {
		name       string
		configPath string
		filePath   string
	}{
		{"custom config path", "/main/sesh.toml", "/main/sesh.toml"},
		{"default config dir", "", "/home/testuser/.config/cc-sesh/cc-sesh.toml"},
	}
	mainTOML := []byte(`import = ["~/imports/rules.toml"]` + "\n")
	for _, rule := range rules {
		for _, cfg := range configs {
			t.Run(rule.name+"/"+cfg.name, func(t *testing.T) {
				mockOs := &testOs{
					homeDir: "/home/testuser",
					files: map[string][]byte{
						cfg.filePath:                        mainTOML,
						"/home/testuser/imports/rules.toml": []byte(rule.importTOML),
					},
				}

				c := NewConfiguratorWithPath(mockOs, pathwrap.NewPath(), &runtimewrap.MockRunTime{}, cfg.configPath)
				_, err := c.GetConfig()

				require.Error(t, err)
				assert.Contains(t, err.Error(), rule.wantErr)
			})
		}
	}
}
