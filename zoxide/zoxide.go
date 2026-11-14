package zoxide

import (
	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/shell"
)

type Zoxide interface {
	ListResults() ([]*model.ZoxideResult, error)
	Add(path string) error
	Query(path string) (*model.ZoxideResult, error)
	Remove(path string) error
}

// Default commands reproduce zoxide's behavior, so an empty [frecency]
// config leaves the backend unchanged.
const (
	defaultListCommand   = "zoxide query --list --score"
	defaultQueryCommand  = "zoxide query {}"
	defaultAddCommand    = "zoxide add {}"
	defaultRemoveCommand = "zoxide remove {}"
)

type RealZoxide struct {
	shell         shell.Shell
	listCommand   string
	queryCommand  string
	addCommand    string
	removeCommand string
}

// NewZoxide builds the frecency backend, falling back to the zoxide
// defaults for any command the frecency config leaves empty.
func NewZoxide(shell shell.Shell, frecency model.FrecencyConfig) Zoxide {
	return &RealZoxide{
		shell:         shell,
		listCommand:   orDefault(frecency.ListCommand, defaultListCommand),
		queryCommand:  orDefault(frecency.QueryCommand, defaultQueryCommand),
		addCommand:    orDefault(frecency.AddCommand, defaultAddCommand),
		removeCommand: orDefault(frecency.RemoveCommand, defaultRemoveCommand),
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
