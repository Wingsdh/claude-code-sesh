package zoxide

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/shell"
	"github.com/stretchr/testify/assert"
)

func TestListResults(t *testing.T) {
	t.Run("runs the default zoxide command when list_command is unset", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{})
		mockShell.EXPECT().
			PrepareCmd("zoxide query --list --score", map[string]string{}).
			Return([]string{"zoxide", "query", "--list", "--score"}, nil)
		mockShell.EXPECT().ListCmd("zoxide", "query", "--list", "--score").Return([]string{
			"100.0 /Users/joshmedeski/Downloads",
			" 82.0 /Users/joshmedeski/c/dotfiles/.config/fish",
			" 73.5 /Users/joshmedeski/c/dotfiles/.config/tmux",
			" 56.0 /Users/joshmedeski/c/sesh/v2",
			" 51.5 /Users/joshmedeski/c/dotfiles/.config/sesh",
			" 48.0 /Users/joshmedeski/c/sesh/main",
		}, nil)
		expected := []*model.ZoxideResult{
			{Path: "/Users/joshmedeski/Downloads", Score: 100.0},
			{Path: "/Users/joshmedeski/c/dotfiles/.config/fish", Score: 82.0},
			{Path: "/Users/joshmedeski/c/dotfiles/.config/tmux", Score: 73.5},
			{Path: "/Users/joshmedeski/c/sesh/v2", Score: 56.0},
			{Path: "/Users/joshmedeski/c/dotfiles/.config/sesh", Score: 51.5},
			{Path: "/Users/joshmedeski/c/sesh/main", Score: 48.0},
		}
		actual, err := zoxide.ListResults()
		assert.Nil(t, err)
		assert.Equal(t, expected, actual)
	})

	t.Run("runs the configured list_command", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{
			ListCommand: "~/bin/dirs --score",
		})
		mockShell.EXPECT().
			PrepareCmd("~/bin/dirs --score", map[string]string{}).
			Return([]string{"/Users/me/bin/dirs", "--score"}, nil)
		mockShell.EXPECT().ListCmd("/Users/me/bin/dirs", "--score").Return([]string{
			"12.0 /Users/me/Workspace/proj",
			"",
		}, nil)
		actual, err := zoxide.ListResults()
		assert.Nil(t, err)
		assert.Equal(t, []*model.ZoxideResult{
			{Path: "/Users/me/Workspace/proj", Score: 12.0},
		}, actual)
	})

	t.Run("treats lines without a leading score as plain paths", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{ListCommand: "dirs"})
		mockShell.EXPECT().PrepareCmd("dirs", map[string]string{}).Return([]string{"dirs"}, nil)
		mockShell.EXPECT().ListCmd("dirs").Return([]string{
			"  9.5 /Users/me/Workspace/visited",
			"/Users/me/Workspace/worktrees/new proj",
			"",
			"/Users/me/Workspace/after-blank",
			"",
		}, nil)
		actual, err := zoxide.ListResults()
		assert.Nil(t, err)
		assert.Equal(t, []*model.ZoxideResult{
			{Path: "/Users/me/Workspace/visited", Score: 9.5},
			{Path: "/Users/me/Workspace/worktrees/new proj", Score: 0},
			{Path: "/Users/me/Workspace/after-blank", Score: 0},
		}, actual)
	})
}
