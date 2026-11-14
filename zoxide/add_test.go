package zoxide

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/shell"
	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	t.Run("substitutes the path into the default zoxide command", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{})
		mockShell.EXPECT().
			PrepareCmd("zoxide add {}", map[string]string{"{}": "/Users/me/my proj"}).
			Return([]string{"zoxide", "add", "/Users/me/my proj"}, nil)
		mockShell.EXPECT().Cmd("zoxide", "add", "/Users/me/my proj").Return("", nil)
		assert.Nil(t, zoxide.Add("/Users/me/my proj"))
	})

	t.Run("substitutes the path into the configured add_command", func(t *testing.T) {
		mockShell := new(shell.MockShell)
		zoxide := NewZoxide(mockShell, model.FrecencyConfig{AddCommand: "fasd -A {}"})
		mockShell.EXPECT().
			PrepareCmd("fasd -A {}", map[string]string{"{}": "/Users/me/proj"}).
			Return([]string{"fasd", "-A", "/Users/me/proj"}, nil)
		mockShell.EXPECT().Cmd("fasd", "-A", "/Users/me/proj").Return("", nil)
		assert.Nil(t, zoxide.Add("/Users/me/proj"))
	})
}
