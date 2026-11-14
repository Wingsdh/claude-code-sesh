package model

type (
	Config struct {
		Cache                bool                 `toml:"cache"`
		StrictMode           bool                 `toml:"strict_mode"`
		ImportPaths          []string             `toml:"import"`
		DefaultSessionConfig DefaultSessionConfig `toml:"default_session"`
		Blacklist            []string             `toml:"blacklist"`
		SessionConfigs       []SessionConfig      `toml:"session"`
		SortOrder            []string             `toml:"sort_order"`
		WindowConfigs        []WindowConfig       `toml:"window"`
		WildcardConfigs      []WildcardConfig     `toml:"wildcard"`
		DirLength            int                  `toml:"dir_length"`
		SeparatorAware       bool                 `toml:"separator_aware"`
		TmuxCommand          string               `toml:"tmux_command"`
		TUI                  TUIConfig            `toml:"tui"`
		Frecency             FrecencyConfig       `toml:"frecency"`
	}
	Evaluation struct {
		StrictMode bool `toml:"strict_mode"`
	}

	DefaultSessionConfig struct {
		// TODO: mention breaking change in v2 release notes
		// StartupScript  string `toml:"startup_script"`
		StartupCommand string   `toml:"startup_command"`
		Tmuxp          string   `toml:"tmuxp"`
		Tmuxinator     string   `toml:"tmuxinator"`
		PreviewCommand string   `toml:"preview_command"`
		Windows        []string `toml:"windows"`
	}

	SessionConfig struct {
		Name                string `toml:"name"`
		Path                string `toml:"path"`
		DisableStartCommand bool   `toml:"disable_startup_command"`
		DefaultSessionConfig
	}

	WindowConfig struct {
		Name          string `toml:"name"`
		StartupScript string `toml:"startup_script"`
		Path          string `toml:"path"`
	}

	TUIConfig struct {
		// TODO: keybindings and more
		ShowIcons   bool   `toml:"show_icons"`
		Prompt      string `toml:"prompt"`
		Placeholder string `toml:"placeholder"`
	}

	// FrecencyConfig overrides the commands sesh runs against its frecency
	// backend (zoxide by default). Empty fields keep the zoxide defaults.
	FrecencyConfig struct {
		// ListCommand enumerates all tracked entries, one path per line,
		// optionally prefixed with a numeric score.
		ListCommand string `toml:"list_command"`
		// QueryCommand resolves a single input to a path; `{}` is replaced
		// with the query.
		QueryCommand string `toml:"query_command"`
		// AddCommand records a path after connecting; `{}` is the path.
		AddCommand string `toml:"add_command"`
		// RemoveCommand drops a path from the backend; `{}` is the path.
		RemoveCommand string `toml:"remove_command"`
	}

	WildcardConfig struct {
		Pattern             string   `toml:"pattern"`
		StartupCommand      string   `toml:"startup_command"`
		DisableStartCommand bool     `toml:"disable_startup_command"`
		PreviewCommand      string   `toml:"preview_command"`
		Windows             []string `toml:"windows"`
	}
)
