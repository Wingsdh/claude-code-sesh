package zoxide

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/shell"
	"github.com/stretchr/testify/assert"
)

func TestRemove(t *testing.T) {
	t.Run("substitutes the path into the default zoxide command", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{})
		mockShell.EXPECT().
			PrepareCmd("zoxide remove {}", map[string]string{"{}": "/Users/me/proj"}).
			Return([]string{"zoxide", "remove", "/Users/me/proj"}, nil)
		mockShell.EXPECT().Cmd("zoxide", "remove", "/Users/me/proj").Return("", nil)
		assert.Nil(t, zoxide.Remove("/Users/me/proj"))
	})

	t.Run("substitutes the path into the configured remove_command", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{RemoveCommand: "fasd -D {}"})
		mockShell.EXPECT().
			PrepareCmd("fasd -D {}", map[string]string{"{}": "/Users/me/proj"}).
			Return([]string{"fasd", "-D", "/Users/me/proj"}, nil)
		mockShell.EXPECT().Cmd("fasd", "-D", "/Users/me/proj").Return("", nil)
		assert.Nil(t, zoxide.Remove("/Users/me/proj"))
	})
}
