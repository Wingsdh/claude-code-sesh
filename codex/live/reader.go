package live

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	claudelive "github.com/Wingsdh/cc-sesh/v2/claude/live"
)

// Pane is the tmux information needed to locate a Codex CLI process.
type Pane struct {
	SessionName string
	PID         int
	Cwd         string
}

type process struct {
	pid, parent int
	command     string
	started     time.Time
}

// Reader finds Codex CLI processes below tmux panes and reads their rollout events.
type Reader struct {
	sessionsDir string
	processes   func() ([]process, error)
	mu          sync.Mutex
	cache       map[int]cachedRollout
}

type cachedRollout struct {
	started time.Time
	rollout rollout
}

func NewReader(home string) *Reader {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		root = filepath.Join(home, ".codex")
	}
	return &Reader{sessionsDir: filepath.Join(root, "sessions"), processes: listProcesses}
}

func listProcesses() ([]process, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,lstart=,comm=").Output()
	if err != nil {
		return nil, err
	}
	var result []process
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		started, e3 := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(fields[2:7], " "), time.Local)
		if e1 == nil && e2 == nil && e3 == nil {
			result = append(result, process{pid: pid, parent: parent, command: strings.Join(fields[7:], " "), started: started})
		}
	}
	return result, nil
}

// Read returns one instance per Codex CLI process, grouped by its owning pane.
// A rollout is used only for state; process ancestry is the source of truth for liveness.
func (r *Reader) Read(panes []Pane) (map[string]claudelive.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]claudelive.Status)
	if len(panes) == 0 {
		return result, nil
	}
	processes, err := r.processes()
	if err != nil {
		return nil, err
	}
	byPID := make(map[int]process, len(processes))
	for _, p := range processes {
		byPID[p.pid] = p
	}
	paneByPID := make(map[int]Pane, len(panes))
	for _, pane := range panes {
		paneByPID[pane.PID] = pane
	}
	type instance struct {
		pane    Pane
		pid     int
		started time.Time
	}
	var instances []instance
	for _, p := range processes {
		if filepath.Base(p.command) != "codex" {
			continue
		}
		for parent, seen := p.parent, map[int]bool{}; parent > 0 && !seen[parent]; {
			seen[parent] = true
			if pane, ok := paneByPID[parent]; ok {
				instances = append(instances, instance{pane: pane, pid: p.pid, started: p.started})
				break
			}
			ancestor, ok := byPID[parent]
			if !ok {
				break
			}
			parent = ancestor.parent
		}
	}
	if len(instances) == 0 {
		return result, nil
	}

	earliest := instances[0].started
	for _, it := range instances[1:] {
		if it.started.Before(earliest) {
			earliest = it.started
		}
	}
	var rollouts []rollout
	allCached := true
	for _, it := range instances {
		cached, ok := r.cache[it.pid]
		if !ok || !cached.started.Equal(it.started) {
			allCached = false
			break
		}
		rollouts = append(rollouts, cached.rollout)
	}
	if !allCached {
		var err error
		rollouts, err = r.recentRollouts(earliest.Add(-time.Minute), false)
		if err != nil {
			return nil, err
		}
		// Resumed threads retain their old dated path. Scan older dates only when
		// the recent dates have no rollout for a live Codex process.
		for _, it := range instances {
			if chooseRollout(rollouts, it.pane.Cwd, it.started, nil) == nil {
				rollouts, err = r.recentRollouts(earliest.Add(-time.Minute), true)
				if err != nil {
					return nil, err
				}
				break
			}
		}
	}
	used := make(map[string]bool)
	for _, it := range instances {
		chosen := chooseRollout(rollouts, it.pane.Cwd, it.started, used)
		status, err := rolloutStatus(chosen)
		if err != nil {
			delete(r.cache, it.pid)
			return nil, err
		}
		if chosen != nil {
			used[chosen.path] = true
			if r.cache == nil {
				r.cache = make(map[int]cachedRollout)
			}
			r.cache[it.pid] = cachedRollout{started: it.started, rollout: *chosen}
		}
		current := result[it.pane.SessionName]
		current.Total++
		current.Busy += status.Busy
		result[it.pane.SessionName] = current
	}
	return result, nil
}

type rollout struct {
	path    string
	cwd     string
	created time.Time
	modTime time.Time
}

func (r *Reader) recentRollouts(since time.Time, includeOld bool) ([]rollout, error) {
	var out []rollout
	err := filepath.WalkDir(r.sessionsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if !includeOld {
				rel, err := filepath.Rel(r.sessionsDir, path)
				if err == nil {
					parts := strings.Split(rel, string(filepath.Separator))
					cutoff := since.Format("2006/01/02")
					prefix := strings.Join(parts, "/")
					if len(parts) <= 3 && prefix != "." && prefix < cutoff[:min(len(prefix), len(cutoff))] {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(since) {
			return nil
		}
		cwd, created, err := rolloutMeta(path)
		if err == nil && cwd != "" {
			out = append(out, rollout{path: path, cwd: claudelive.NormalizeCwd(cwd), created: created, modTime: info.ModTime()})
		}
		return nil
	})
	return out, err
}

func rolloutMeta(path string) (string, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, err
	}
	defer f.Close()
	var item struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd       string    `json:"cwd"`
			Timestamp time.Time `json:"timestamp"`
		} `json:"payload"`
	}
	err = json.NewDecoder(f).Decode(&item)
	if err != nil || item.Type != "session_meta" {
		return "", time.Time{}, err
	}
	return item.Payload.Cwd, item.Payload.Timestamp, nil
}

func chooseRollout(rollouts []rollout, cwd string, started time.Time, used map[string]bool) *rollout {
	var chosen *rollout
	cwd = claudelive.NormalizeCwd(cwd)
	for i := range rollouts {
		candidate := &rollouts[i]
		if candidate.cwd != cwd || candidate.modTime.Before(started.Add(-time.Minute)) || used[candidate.path] {
			continue
		}
		if chosen == nil || rolloutScore(*candidate, started) < rolloutScore(*chosen, started) {
			chosen = candidate
		}
	}
	return chosen
}

func rolloutScore(candidate rollout, started time.Time) time.Duration {
	if !candidate.created.IsZero() {
		delta := candidate.created.Sub(started)
		if delta < 0 {
			delta = -delta
		}
		if delta < time.Minute {
			return delta
		}
	}
	return time.Hour + time.Since(candidate.modTime)
}

func rolloutStatus(chosen *rollout) (claudelive.Status, error) {
	if chosen == nil {
		return claudelive.Status{}, nil
	}
	busy, err := rolloutBusy(chosen.path)
	if err != nil {
		return claudelive.Status{}, err
	}
	if !busy {
		return claudelive.Status{}, nil
	}
	return claudelive.Status{Busy: 1}, nil
}

func rolloutBusy(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	busy := false
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var item struct {
				Type    string `json:"type"`
				Payload struct {
					Type string `json:"type"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &item) == nil && item.Type == "event_msg" {
				switch item.Payload.Type {
				case "task_started", "turn_started":
					busy = true
				case "task_complete", "turn_complete", "turn_aborted":
					busy = false
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return busy, nil
			}
			return false, err
		}
	}
}
