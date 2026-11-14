package namer

import (
	"fmt"
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/git"
	"github.com/Wingsdh/cc-sesh/v2/home"
	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/pathwrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const worktrees = "/Users/josh/Workspace/worktrees"

var stripWorktrees = []model.NameSubstitution{{Find: "~/Workspace/worktrees/", Replace: ""}}

func TestNameAppliesNameSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		short    string
		expected string
	}{
		{"worktree dir", worktrees + "/nodemicro", "~/Workspace/worktrees/nodemicro", "nodemicro"},
		{"subdir of a worktree", worktrees + "/nodemicro/src", "~/Workspace/worktrees/nodemicro/src", "nodemicro/src"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPathwrap := new(pathwrap.MockPath)
			mockGit := new(git.MockGit)
			mockHome := new(home.MockHome)
			mockPathwrap.On("EvalSymlinks", tt.path).Return(tt.path, nil)
			mockHome.On("ShortenHome", tt.path).Return(tt.short, nil)
			n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: stripWorktrees})

			name, err := n.Name(tt.path)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, name)
			mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
		})
	}
}

func TestRootNameAppliesNameSubstitution(t *testing.T) {
	path := worktrees + "/nodemicro"
	mockPathwrap := new(pathwrap.MockPath)
	mockGit := new(git.MockGit)
	mockHome := new(home.MockHome)
	mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
	mockHome.On("ShortenHome", path).Return("~/Workspace/worktrees/nodemicro", nil)
	n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: stripWorktrees})

	name, err := n.RootName(path)

	assert.NoError(t, err)
	assert.Equal(t, "nodemicro", name)
	mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
}

func TestNameSubstitutionRulesComposeInOrder(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		short    string
		rules    []model.NameSubstitution
		expected string
	}{
		{
			name:  "each rule works on the previous rule's output",
			path:  worktrees + "/nodemicro",
			short: "~/Workspace/worktrees/nodemicro",
			rules: []model.NameSubstitution{
				{Find: "~/Workspace/worktrees/", Replace: "wt:"},
				{Find: "wt:", Replace: "w-"},
			},
			expected: "w-nodemicro",
		},
		{
			name:     "a rule replaces every occurrence",
			path:     "/Users/josh/work/work/api",
			short:    "~/work/work/api",
			rules:    []model.NameSubstitution{{Find: "work/", Replace: ""}},
			expected: "~/api",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPathwrap := new(pathwrap.MockPath)
			mockGit := new(git.MockGit)
			mockHome := new(home.MockHome)
			mockPathwrap.On("EvalSymlinks", tt.path).Return(tt.path, nil)
			mockHome.On("ShortenHome", tt.path).Return(tt.short, nil)
			n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: tt.rules})

			name, err := n.Name(tt.path)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestNameSubstitutionFallsThroughWhenNoRuleMatches(t *testing.T) {
	gitList := `worktree /Users/josh/config/dotfiles
HEAD abc123
branch refs/heads/main
`
	config := model.Config{DirLength: 1, NameSubstitutions: stripWorktrees}

	t.Run("Name uses the git strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/.config/neovim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/config/dotfiles/.config/neovim", nil)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles/_config/neovim", name)
	})

	t.Run("Name uses the dir strategy outside git", func(t *testing.T) {
		path := "/Users/josh/.config/neovim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/.config/neovim", nil)
		mockGit.On("WorktreeList", path).Return(false, "", fmt.Errorf("not a git repository"))
		mockPathwrap.On("Base", path).Return("neovim")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "neovim", name)
	})

	t.Run("RootName uses the git root strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/.config/neovim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/config/dotfiles/.config/neovim", nil)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockGit.On("ShowTopLevel", path).Return(true, "/Users/josh/config/dotfiles", nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.RootName(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles", name)
	})
}

func TestNameSubstitutionResultIsMadeValid(t *testing.T) {
	path := worktrees + "/node.micro"
	mockPathwrap := new(pathwrap.MockPath)
	mockGit := new(git.MockGit)
	mockHome := new(home.MockHome)
	mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
	mockHome.On("ShortenHome", path).Return("~/Workspace/worktrees/node.micro", nil)
	rules := []model.NameSubstitution{{Find: "~/Workspace/worktrees/", Replace: "wt "}}
	n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

	name, err := n.Name(path)

	assert.NoError(t, err)
	// "wt node.micro": the space and the dot are legalized like any other name.
	assert.Equal(t, "wt_node_micro", name)
}

func TestNameSubstitutionSkipsRuleWithEmptyFind(t *testing.T) {
	path := worktrees + "/nodemicro"
	mockPathwrap := new(pathwrap.MockPath)
	mockGit := new(git.MockGit)
	mockHome := new(home.MockHome)
	mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
	mockHome.On("ShortenHome", path).Return("~/Workspace/worktrees/nodemicro", nil)
	rules := []model.NameSubstitution{
		{Find: "", Replace: "x"},
		{Find: "~/Workspace/worktrees/", Replace: ""},
	}
	n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

	name, err := n.Name(path)

	assert.NoError(t, err)
	assert.Equal(t, "nodemicro", name)
}
