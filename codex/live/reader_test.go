package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	claudelive "github.com/Wingsdh/cc-sesh/v2/claude/live"
)

func writeRollout(t *testing.T, home, cwd string, events ...string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "10", "08")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "rollout-test.jsonl")
	var lines []byte
	for _, item := range append([]string{"session_meta"}, events...) {
		payload := map[string]string{"type": item}
		if item == "session_meta" {
			payload["cwd"] = cwd
		}
		kind := "event_msg"
		if item == "session_meta" {
			kind = item
		}
		line, err := json.Marshal(map[string]any{"type": kind, "payload": payload})
		require.NoError(t, err)
		lines = append(lines, append(line, '\n')...)
	}
	require.NoError(t, os.WriteFile(path, lines, 0o644))
	return path
}

func TestReadMatchesCodexProcessToPaneAndTracksTurn(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "/work/project", "task_started")
	started := time.Now().Add(-time.Minute)
	r := NewReader(home)
	r.processes = func() ([]process, error) {
		return []process{
			{pid: 10, parent: 1, command: "zsh", started: started},
			{pid: 11, parent: 10, command: "codex", started: started},
			{pid: 20, parent: 1, command: "zsh", started: started},
			{pid: 21, parent: 20, command: "claude", started: started},
		}, nil
	}
	panes := []Pane{{SessionName: "alpha", PID: 10, Cwd: "/work/project"}, {SessionName: "beta", PID: 20, Cwd: "/work/project"}}
	got, err := r.Read(panes)
	require.NoError(t, err)
	require.Equal(t, claudelive.Status{Total: 1, Busy: 1}, got["alpha"])
	require.NotContains(t, got, "beta")

	writeRollout(t, home, "/work/project", "task_started", "task_complete")
	got, err = r.Read(panes)
	require.NoError(t, err)
	require.Equal(t, claudelive.Status{Total: 1}, got["alpha"])

	writeRollout(t, home, "/work/project", "turn_started", "turn_aborted")
	got, err = r.Read(panes)
	require.NoError(t, err)
	require.Equal(t, claudelive.Status{Total: 1}, got["alpha"])
}

func TestChooseRolloutPrefersProcessStartOverUnrelatedNewerThread(t *testing.T) {
	started := time.Now().Add(-time.Hour)
	items := []rollout{
		{path: "own", cwd: "/work/project", created: started.Add(2 * time.Second), modTime: started.Add(10 * time.Second)},
		{path: "other", cwd: "/work/project", created: started.Add(20 * time.Minute), modTime: time.Now()},
	}
	chosen := chooseRollout(items, "/work/project", started, nil)
	require.NotNil(t, chosen)
	require.Equal(t, "own", chosen.path)
	chosen = chooseRollout(items, "/work/project", started, map[string]bool{"own": true})
	require.NotNil(t, chosen)
	require.Equal(t, "other", chosen.path)
}

func TestReadFindsResumedRolloutInOldDateDirectory(t *testing.T) {
	home := t.TempDir()
	path := writeRollout(t, home, "/work/project", "task_started")
	oldDir := filepath.Join(home, ".codex", "sessions", "2025", "01", "01")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.Rename(path, filepath.Join(oldDir, filepath.Base(path))))
	r := NewReader(home)
	r.processes = func() ([]process, error) {
		return []process{{pid: 11, parent: 10, command: "codex", started: time.Now().Add(-time.Minute)}}, nil
	}
	got, err := r.Read([]Pane{{SessionName: "alpha", PID: 10, Cwd: "/work/project"}})
	require.NoError(t, err)
	require.Equal(t, claudelive.Status{Total: 1, Busy: 1}, got["alpha"])
}

func TestReadWithoutProcessDoesNotShowStaleRollout(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "/work/project", "task_started")
	r := NewReader(home)
	r.processes = func() ([]process, error) { return nil, nil }
	got, err := r.Read([]Pane{{SessionName: "alpha", PID: 10, Cwd: "/work/project"}})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReadNoRolloutStillCountsLiveCodex(t *testing.T) {
	r := NewReader(t.TempDir())
	r.processes = func() ([]process, error) {
		return []process{{pid: 11, parent: 10, command: "/opt/bin/codex", started: time.Now()}}, nil
	}
	got, err := r.Read([]Pane{{SessionName: "alpha", PID: 10, Cwd: "/work/project"}})
	require.NoError(t, err)
	require.Equal(t, claudelive.Status{Total: 1}, got["alpha"])
}
