package namer

import (
	"testing"

	"github.com/Wingsdh/cc-sesh/v2/git"
	"github.com/Wingsdh/cc-sesh/v2/home"
	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/pathwrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// gitRepoCase is a path inside a git repo together with what git would report
// for it: the `git worktree list --porcelain` output and the
// `git rev-parse --show-toplevel` result.
type gitRepoCase struct {
	name     string
	path     string
	rootPath string // main working tree the git strategy would name from
	list     string
	topLevel string
	folder   string // the path's base name
}

var gitRepoCases = []gitRepoCase{
	{
		name:     "worktree dir inside a repo",
		path:     "/Users/josh/Workspace/worktrees/nodemicro",
		rootPath: "/Users/josh/Workspace/worktrees",
		list:     "worktree /Users/josh/Workspace/worktrees\nHEAD abc123\nbranch refs/heads/main\n",
		topLevel: "/Users/josh/Workspace/worktrees",
		folder:   "nodemicro",
	},
	{
		name:     "subdir of a repo",
		path:     "/Users/josh/config/dotfiles/.config/neovim",
		rootPath: "/Users/josh/config/dotfiles",
		list:     "worktree /Users/josh/config/dotfiles\nHEAD abc123\nbranch refs/heads/main\n",
		topLevel: "/Users/josh/config/dotfiles",
		folder:   "neovim",
	},
	{
		name:     "worktree of a bare repo",
		path:     "/Users/josh/projects/sesh/main",
		rootPath: "/Users/josh/projects/sesh",
		list:     "worktree /Users/josh/projects/sesh/.bare\nbare\n\nworktree /Users/josh/projects/sesh/main\nHEAD ba04ca494\nbranch refs/heads/main\n",
		topLevel: "/Users/josh/projects/sesh/main",
		folder:   "main",
	},
}

// newGitRepoNamer builds a namer whose git mock reports tc as a git repo, so
// the git strategies would produce a "<repo>/..." name if they ran.
func newGitRepoNamer(tc gitRepoCase, config model.Config) (Namer, *git.MockGit) {
	mockPathwrap := new(pathwrap.MockPath)
	mockGit := new(git.MockGit)
	mockHome := new(home.MockHome)
	mockPathwrap.On("EvalSymlinks", tc.path).Return(tc.path, nil)
	mockPathwrap.On("Base", tc.path).Return(tc.folder)
	mockPathwrap.On("Base", tc.rootPath).Return("repo")
	mockGit.On("WorktreeList", tc.path).Return(true, tc.list, nil)
	mockGit.On("ShowTopLevel", tc.path).Return(true, tc.topLevel, nil)
	return NewNamer(mockPathwrap, mockGit, mockHome, config), mockGit
}

func TestNameWithGitNamerDisabledUsesFolderName(t *testing.T) {
	for _, tc := range gitRepoCases {
		t.Run(tc.name, func(t *testing.T) {
			n, mockGit := newGitRepoNamer(tc, model.Config{DirLength: 1, DisableGitNamer: true})

			name, err := n.Name(tc.path)

			assert.NoError(t, err)
			assert.Equal(t, tc.folder, name)
			mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
		})
	}
}

func TestNameWithGitNamerDisabledHonorsDirLength(t *testing.T) {
	tc := gitRepoCases[1] // /Users/josh/config/dotfiles/.config/neovim
	n, mockGit := newGitRepoNamer(tc, model.Config{DirLength: 2, DisableGitNamer: true})

	name, err := n.Name(tc.path)

	assert.NoError(t, err)
	assert.Equal(t, "_config/neovim", name)
	mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
}

func TestRootNameWithGitNamerDisabledUsesFolderName(t *testing.T) {
	for _, tc := range gitRepoCases {
		t.Run(tc.name, func(t *testing.T) {
			n, mockGit := newGitRepoNamer(tc, model.Config{DirLength: 1, DisableGitNamer: true})

			name, err := n.RootName(tc.path)

			assert.NoError(t, err)
			assert.Equal(t, tc.folder, name)
			mockGit.AssertNotCalled(t, "WorktreeList", mock.Anything)
			mockGit.AssertNotCalled(t, "ShowTopLevel", mock.Anything)
		})
	}
}

func TestNameSubstitutionStillRunsFirstWithGitNamerDisabled(t *testing.T) {
	path := worktrees + "/nodemicro/src"
	config := model.Config{DirLength: 1, DisableGitNamer: true, NameSubstitutions: stripWorktrees}
	methods := []struct {
		name string
		call func(Namer, string) (string, error)
	}{
		{"Name", Namer.Name},
		{"RootName", Namer.RootName},
	}
	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			mockPathwrap := new(pathwrap.MockPath)
			mockGit := new(git.MockGit)
			mockHome := new(home.MockHome)
			mockPathwrap.On("EvalSymlinks", path).Return(path, nil)
			mockPathwrap.On("Base", path).Return("src")
			mockHome.On("ShortenHome", path).Return("~/Workspace/worktrees/nodemicro/src", nil)
			n := NewNamer(mockPathwrap, mockGit, mockHome, config)

			name, err := m.call(n, path)

			assert.NoError(t, err)
			assert.Equal(t, "nodemicro/src", name)
		})
	}
}
