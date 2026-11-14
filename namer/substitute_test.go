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

func TestNameAppliesRegexNameSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		short    string
		rule     model.NameSubstitution
		expected string
	}{
		{
			name:     "replace references a capture group",
			path:     worktrees + "/12_nodemicro",
			short:    "~/Workspace/worktrees/12_nodemicro",
			rule:     model.NameSubstitution{Find: `^~/Workspace/worktrees/[0-9]+_(.*)$`, Replace: "wt-$1", Regex: true},
			expected: "wt-nodemicro",
		},
		{
			name:     "every match is replaced",
			path:     "/Users/josh/v1/api2",
			short:    "~/v1/api2",
			rule:     model.NameSubstitution{Find: `[0-9]+`, Replace: "N", Regex: true},
			expected: "~/vN/apiN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPathwrap := new(pathwrap.MockPath)
			mockGit := new(git.MockGit)
			mockHome := new(home.MockHome)
			mockPathwrap.On("EvalSymlinks", tt.path).Return(tt.path, nil)
			mockHome.On("ShortenHome", tt.path).Return(tt.short, nil)
			rules := []model.NameSubstitution{tt.rule}
			n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

			name, err := n.Name(tt.path)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, name)
			mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
		})
	}
}

func TestNameSubstitutionMatchesMetacharactersLiterallyUnlessRegex(t *testing.T) {
	path := "/Users/josh/c/dotXconfig/nvim"
	short := "~/c/dotXconfig/nvim"

	t.Run("literal rule leaves the path alone and falls through", func(t *testing.T) {
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return(short, nil)
		mockGit.On("WorktreeList", path).Return(false, "", fmt.Errorf("not a git repository"))
		mockPathwrap.On("Base", path).Return("nvim")
		rules := []model.NameSubstitution{{Find: "~/c/dot.config/", Replace: "cfg-"}}
		n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

		name, err := n.Name(path)

		assert.NoError(t, err)
		// "." only matches a literal dot, so the rule misses and the dir
		// strategy names the session.
		assert.Equal(t, "nvim", name)
	})

	t.Run("regex rule treats the dot as a wildcard", func(t *testing.T) {
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return(short, nil)
		rules := []model.NameSubstitution{{Find: "~/c/dot.config/", Replace: "cfg-", Regex: true}}
		n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "cfg-nvim", name)
		mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
	})
}

func TestNameSubstitutionMixesRegexAndLiteralRulesInOrder(t *testing.T) {
	path := worktrees + "/12_nodemicro"
	short := "~/Workspace/worktrees/12_nodemicro"
	stripPrefix := model.NameSubstitution{Find: "~/Workspace/worktrees/", Replace: ""}
	swapTicket := model.NameSubstitution{Find: `^([0-9]+)_(.*)$`, Replace: "$2-$1", Regex: true}
	tests := []struct {
		name     string
		rules    []model.NameSubstitution
		expected string
	}{
		{
			name:     "literal then regex: the regex sees the stripped path",
			rules:    []model.NameSubstitution{stripPrefix, swapTicket},
			expected: "nodemicro-12",
		},
		{
			name:     "regex then literal: the anchored regex misses the full path",
			rules:    []model.NameSubstitution{swapTicket, stripPrefix},
			expected: "12_nodemicro",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPathwrap := new(pathwrap.MockPath)
			mockGit := new(git.MockGit)
			mockHome := new(home.MockHome)
			mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
			mockHome.On("ShortenHome", path).Return(short, nil)
			n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: tt.rules})

			name, err := n.Name(path)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestNameSubstitutionSkipsRuleWithInvalidRegex(t *testing.T) {
	path := worktrees + "/nodemicro"
	mockPathwrap := new(pathwrap.MockPath)
	mockGit := new(git.MockGit)
	mockHome := new(home.MockHome)
	mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
	mockHome.On("ShortenHome", path).Return("~/Workspace/worktrees/nodemicro", nil)
	rules := []model.NameSubstitution{
		{Find: "(unterminated", Replace: "x", Regex: true},
		{Find: "~/Workspace/worktrees/", Replace: ""},
	}
	n := NewNamer(mockPathwrap, mockGit, mockHome, model.Config{DirLength: 1, NameSubstitutions: rules})

	name, err := n.Name(path)

	assert.NoError(t, err)
	assert.Equal(t, "nodemicro", name)
}

func TestNameSubstitutionThatLegalizesToEmptyFallsThrough(t *testing.T) {
	gitList := `worktree /Users/josh/config/dotfiles
HEAD abc123
branch refs/heads/main
`
	// The rule turns the whole path into whitespace, which legalizes to an
	// empty name, so the next strategy has to name the session instead.
	blankOut := []model.NameSubstitution{{Find: `^.*$`, Replace: " \t ", Regex: true}}
	config := model.Config{DirLength: 1, NameSubstitutions: blankOut}

	t.Run("Name uses the git strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/nvim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/config/dotfiles/nvim", nil)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles/nvim", name)
	})

	t.Run("Name uses the dir strategy outside git", func(t *testing.T) {
		path := "/Users/josh/scratch/notes"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/scratch/notes", nil)
		mockGit.On("WorktreeList", path).Return(false, "", fmt.Errorf("not a git repository"))
		mockPathwrap.On("Base", path).Return("notes")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "notes", name)
	})

	t.Run("RootName uses the git root strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/nvim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("~/config/dotfiles/nvim", nil)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockGit.On("ShowTopLevel", path).Return(true, "/Users/josh/config/dotfiles", nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.RootName(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles", name)
	})
}

func TestNameSubstitutionFallsThroughWhenHomeIsUnavailable(t *testing.T) {
	gitList := `worktree /Users/josh/config/dotfiles
HEAD abc123
branch refs/heads/main
`
	noHome := fmt.Errorf("$HOME is not defined")
	config := model.Config{DirLength: 1, NameSubstitutions: stripWorktrees}

	t.Run("Name uses the git strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/nvim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("", noHome)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles/nvim", name)
	})

	t.Run("Name uses the dir strategy outside git", func(t *testing.T) {
		path := worktrees + "/nodemicro"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("", noHome)
		mockGit.On("WorktreeList", path).Return(false, "", fmt.Errorf("not a git repository"))
		mockPathwrap.On("Base", path).Return("nodemicro")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.Name(path)

		assert.NoError(t, err)
		assert.Equal(t, "nodemicro", name)
	})

	t.Run("RootName uses the git root strategy", func(t *testing.T) {
		path := "/Users/josh/config/dotfiles/nvim"
		mockPathwrap := new(pathwrap.MockPath)
		mockGit := new(git.MockGit)
		mockHome := new(home.MockHome)
		mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
		mockHome.On("ShortenHome", path).Return("", noHome)
		mockGit.On("WorktreeList", path).Return(true, gitList, nil)
		mockGit.On("ShowTopLevel", path).Return(true, "/Users/josh/config/dotfiles", nil)
		mockPathwrap.On("Base", "/Users/josh/config/dotfiles").Return("dotfiles")
		n := NewNamer(mockPathwrap, mockGit, mockHome, config)

		name, err := n.RootName(path)

		assert.NoError(t, err)
		assert.Equal(t, "dotfiles", name)
	})
}
