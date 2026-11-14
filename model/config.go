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
		NameSubstitutions    []NameSubstitution   `toml:"name_substitution"`
		SeparatorAware       bool                 `toml:"separator_aware"`
		TmuxCommand          string               `toml:"tmux_command"`
		TUI                  TUIConfig            `toml:"tui"`
	}
	Evaluation struct {
		StrictMode bool `toml:"strict_mode"`
	}

	// NameSubstitution rewrites the path a session name is derived from before
	// it becomes a tmux session name, so a deeply nested directory can be shown
	// under a short, meaningful name (e.g. "~/Workspace/worktrees/nodemicro" as
	// "nodemicro"). Rules run against the home-collapsed path (a leading "~")
	// ahead of the git and directory strategies. When no rule changes the path,
	// naming falls back to those strategies, so an empty or absent
	// [[name_substitution]] list leaves behavior unchanged.
	NameSubstitution struct {
		// Find is the text to look for, matched literally.
		Find string `toml:"find"`
		// Replace is the text every occurrence of Find is replaced with.
		Replace string `toml:"replace"`
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

	WildcardConfig struct {
		Pattern             string   `toml:"pattern"`
		StartupCommand      string   `toml:"startup_command"`
		DisableStartCommand bool     `toml:"disable_startup_command"`
		PreviewCommand      string   `toml:"preview_command"`
		Windows             []string `toml:"windows"`
	}
)
