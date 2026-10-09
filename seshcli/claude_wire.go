package seshcli

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/Wingsdh/cc-sesh/v2/claude/attention"
	"github.com/Wingsdh/cc-sesh/v2/claude/live"
	codexlive "github.com/Wingsdh/cc-sesh/v2/codex/live"
	"github.com/Wingsdh/cc-sesh/v2/lister"
	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/picker"
	"github.com/Wingsdh/cc-sesh/v2/tmux"
)

// makeClaudeFetcher 把 lister.List + claude/live + claude/attention 串成一个 picker.FetchFunc。
//
// 支持运行时切换 mode：
//   - all     ：用调用方传入的 listerOpts（默认行为）
//   - tmux    ：仅 tmux session
//   - config  ：仅 sesh.toml 配置 session
//   - zoxide  ：仅 zoxide 历史目录
//   - find    ：用 filepath.WalkDir 在 home 下深度 ≤2 列目录（替代 fzf 路径里的 fd）
//
// 任何一步失败都不阻塞 picker —— 走 fallback（无 live / 无 attention）继续。
func makeClaudeFetcher(deps *Deps, listerOpts lister.ListOptions) picker.FetchFunc {
	return func(mode string) (picker.FetchResult, error) {
		if mode == picker.ModeFind {
			return fetchFindResults(deps)
		}

		opts := listerOpts
		switch mode {
		case picker.ModeTmux:
			opts = lister.ListOptions{Tmux: true}
		case picker.ModeConfig:
			opts = lister.ListOptions{Config: true}
		case picker.ModeZoxide:
			opts = lister.ListOptions{Zoxide: true}
		}

		sessions, err := deps.Lister.List(opts)
		if err != nil {
			return picker.FetchResult{}, err
		}

		instances, instancesOk := readInstancesOrEmpty(deps.LiveReader)
		agentInfo := agentSnapshot{}
		liveByName, aggregateOk := aggregateBySession(instances, deps.Tmux, deps.CodexReader, &agentInfo)
		liveOk := instancesOk && aggregateOk

		flags := reconcileAttention(deps.Attention, deps.Tmux, sessions, liveByName, liveOk)

		agents, windowAgents := agentInfo.sessions, agentInfo.windows
		windows := fetchWindowItems(mode, deps.Tmux)
		for i := range windows {
			windows[i].Agents = windowAgents[windowAgentKey(windows[i].SessionName, windows[i].Index)]
		}
		return picker.FetchResult{
			Sessions: sessions,
			Decorator: &claudeDecorator{
				liveByName: liveByName,
				agents:     agents,
				flags:      flags,
			},
			Windows: windows,
		}, nil
	}
}

// fetchWindowItems 只在 all / tmux 模式下拉全量 window 清单——
// 其余模式（config/zoxide/find）的条目本来就不可展开，拉了也没人用，
// 白白多跑一次 tmux 命令。
//
// fail-soft：ListAllWindows 报错时只 warn 并返回 nil，让 picker 退化成
// 「全部 session 不可展开」，绝不阻断整个取数（与 live / attention 一致）。
func fetchWindowItems(mode string, t tmux.Tmux) []picker.WindowItem {
	if mode != picker.ModeAll && mode != picker.ModeTmux {
		return nil
	}
	if t == nil {
		return nil
	}
	raw, err := t.ListAllWindows()
	if err != nil {
		slog.Warn("claude: list all windows failed", "error", err)
		return nil
	}
	items := make([]picker.WindowItem, 0, len(raw))
	for _, w := range raw {
		if w == nil {
			continue
		}
		items = append(items, picker.WindowItem{
			SessionName: w.SessionName,
			Index:       w.Index,
			Name:        w.Name,
			Active:      w.Active,
		})
	}
	return items
}

// fetchFindResults 用 filepath.WalkDir 列 home 下深度 ≤2 的目录，
// 对应 fzf 路径里的 `fd -H -d 2 -t d -E .Trash . ~` 行为。
// 不依赖外部 fd，跨环境通用。
func fetchFindResults(deps *Deps) (picker.FetchResult, error) {
	home, err := deps.Os.UserHomeDir()
	if err != nil {
		return picker.FetchResult{Decorator: picker.NoDecoration{}}, err
	}
	dir := make(model.SeshSessionMap)
	index := []string{}
	const maxDepth = 2
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path == home {
			return nil
		}
		base := filepath.Base(path)
		if base == ".Trash" || strings.HasPrefix(base, ".") && base != "." {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(home, path)
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		key := "find:" + path
		index = append(index, key)
		dir[key] = model.SeshSession{
			Src:  "find",
			Name: path,
			Path: path,
		}
		return nil
	})
	return picker.FetchResult{
		Sessions:  model.SeshSessions{Directory: dir, OrderedIndex: index},
		Decorator: picker.NoDecoration{},
	}, nil
}

// readInstancesOrEmpty 返回 live 实例切片和"读取是否成功"标志。
// ok=false 表示瞬时读失败 —— 调用方应把这视作"live 数据不可信"，
// 不能据此推断"某 session 的 cc 消失了"（否则会清掉合法 tracking）。
func readInstancesOrEmpty(r *live.Reader) (items []live.Instance, ok bool) {
	if r == nil {
		// 没有 reader 配置等价于"系统里就没有 cc"，是合法 empty
		return nil, true
	}
	items, err := r.ReadInstances()
	if err != nil {
		slog.Warn("claude: live read failed", "error", err)
		return nil, false
	}
	return items, true
}

// aggregateBySession 返回 cwd→session 聚合后的状态，以及"聚合数据是否可信"。
// 任何一步失败（tmux 不在 / ListAllPanes 失败）→ ok=false，调用方不应据此清 tracking。
type codexStatusReader interface {
	Read([]codexlive.Pane) (map[string]live.Status, error)
}

func aggregateBySession(instances []live.Instance, t tmux.Tmux, codexReader codexStatusReader, snapshots ...*agentSnapshot) (map[string]live.Status, bool) {
	if t == nil {
		return nil, false
	}
	rawPanes, err := t.ListAllPanes()
	if err != nil {
		slog.Warn("claude: list panes failed", "error", err)
		return nil, false
	}
	paneInfos := make([]live.PaneInfo, 0, len(rawPanes))
	codexPanes := make([]codexlive.Pane, 0, len(rawPanes))
	for _, p := range rawPanes {
		if p == nil {
			continue
		}
		paneInfos = append(paneInfos, live.PaneInfo{
			SessionName: p.SessionName,
			Cwd:         p.PaneCurrentPath,
		})
		codexPanes = append(codexPanes, codexlive.Pane{
			SessionName: windowAgentKey(p.SessionName, p.WindowIndex),
			PID:         p.PanePID,
			Cwd:         p.PaneCurrentPath,
		})
	}

	combined := live.AggregateBySession(instances, paneInfos)
	info := agentSnapshot{sessions: map[string]picker.AgentBadge{}, windows: map[string]picker.AgentBadge{}}
	parents := map[int]int{}
	if len(instances) > 0 {
		parents = processParents()
	}
	for _, p := range rawPanes {
		if p == nil {
			continue
		}
		badge := picker.AgentBadge{}
		for _, it := range instances {
			if isDescendant(it.PID, p.PanePID, parents) {
				badge.CC = true
				break
			}
		}
		info.sessions[p.SessionName] = info.sessions[p.SessionName].Merge(badge)
		key := windowAgentKey(p.SessionName, p.WindowIndex)
		info.windows[key] = info.windows[key].Merge(badge)
	}
	if codexReader != nil {
		// Key by window so one process snapshot supplies both window and session badges.
		codexByWindow, err := codexReader.Read(codexPanes)
		if err != nil {
			slog.Warn("codex: live read failed", "error", err)
			return combined, false
		}
		seen := map[string]bool{}
		for _, p := range rawPanes {
			if p == nil {
				continue
			}
			key := windowAgentKey(p.SessionName, p.WindowIndex)
			if seen[key] {
				continue
			}
			seen[key] = true
			status := codexByWindow[key]
			mergeLiveStatus(combined, map[string]live.Status{p.SessionName: status})
			badge := picker.AgentBadge{CX: status.Total > 0}
			info.sessions[p.SessionName] = info.sessions[p.SessionName].Merge(badge)
			info.windows[key] = info.windows[key].Merge(badge)
		}
	}
	for _, target := range snapshots {
		*target = info
	}

	return combined, true
}

func mergeLiveStatus(into, additional map[string]live.Status) {
	for name, status := range additional {
		current := into[name]
		current.Total += status.Total
		current.Busy += status.Busy
		current.Subagent += status.Subagent
		current.Needing += status.Needing
		into[name] = current
	}
}

// reconcileAttention 调度 live 数据 + tmux client 信息更新 attention store。
//
// liveOk=false 时 live 数据不可信（瞬时读失败），这一轮只跑 suppress 路径
// （清掉当前 attach session 的已有 flag），跳过任何会动 tracking 的逻辑——
// 否则会把"看不见 cc"误判为"cc 消失了"，把合法 tracking 清掉，导致后续
// cc 真的完成时无法触发 flag。
func reconcileAttention(
	store *attention.Store,
	t tmux.Tmux,
	sessions model.SeshSessions,
	liveByName map[string]live.Status,
	liveOk bool,
) map[string]attention.Flag {
	if store == nil {
		return nil
	}
	busyByName := map[string]bool{}
	// activeNames 只在 live 可信时填充：传 nil 给 Reconcile 会跳过
	// "cc disappeared 清 tracking" 和 "GC dead session" 两段——
	// 前者在 live 不可信时必须跳过；后者顺便跳过没大碍，等下一轮再 GC。
	var activeNames []string
	if liveOk {
		activeNames = make([]string, 0, len(sessions.Directory))
	}
	for _, key := range sessions.OrderedIndex {
		s := sessions.Directory[key]
		// 只对真实 tmux session 跟踪 attention：其他 src 还没起 session 无法 attach 清除
		if s.Src != "tmux" {
			continue
		}
		if liveOk {
			activeNames = append(activeNames, s.Name)
		}
		// 只把"有 live cc 实例"的 session 写进 busyByName；
		// cc 消失的 session 不写，Store 会清掉 tracking 不触发 flag。
		st, ok := liveByName[s.Name]
		if !ok {
			continue
		}
		// busy/subagent 算「在跑活」；needs-input 不算（用户拒绝/忽略不该算"完成"）
		busyByName[s.Name] = st.Busy+st.Subagent > 0
	}

	// 取所有当前被 client attach 的 session 作为 suppress 集合：
	// 这些 session 不触发 flag、清掉已存在 flag（用户正在看）。
	var suppress []string
	if t != nil {
		if names, err := t.ListClients(); err != nil {
			slog.Warn("claude: list tmux clients failed", "error", err)
		} else {
			suppress = names
		}
	}

	if err := store.Reconcile(busyByName, activeNames, suppress); err != nil {
		slog.Warn("claude: attention reconcile failed", "error", err)
	}
	return store.Load()
}

// claudeDecorator：只对真实存在的 tmux session（src=tmux）显示徽章和 attention。
// zoxide / config / tmuxinator 模板等"还没起 session"的 entry 不显示——
// 因为徽章语义是"这个 session 内有 Claude"，没 session 时贴徽章会与
// 真实 tmux session 重复，且 attention 也无法被 attach 清除。
type claudeDecorator struct {
	agents     map[string]picker.AgentBadge
	liveByName map[string]live.Status
	flags      map[string]attention.Flag
}

func (d *claudeDecorator) Decorate(s model.SeshSession) picker.Decoration {
	var dec picker.Decoration
	if s.Src != "tmux" {
		return dec
	}

	dec.Agents = d.agents[s.Name]
	if st, ok := d.liveByName[s.Name]; ok && st.Total > 0 {
		dec.Live = picker.LiveBadge{
			Total:    st.Total,
			Busy:     st.Busy,
			Subagent: st.Subagent,
			Needing:  st.Needing,
		}
	}

	if f, ok := d.flags[s.Name]; ok {
		dec.Attention = picker.AttentionBadge{
			Triggered: true,
			FirstAt:   f.FirstAt,
		}
	}
	return dec
}

// claudeDismisser 把 attention.Store 适配为 picker.Dismisser，便于 alt+d 手动清除。
type claudeDismisser struct {
	store *attention.Store
}

func (d *claudeDismisser) Dismiss(name string) error {
	if d.store == nil {
		return nil
	}
	return d.store.Ack(name)
}

// tmuxCapturer 把 tmux.CapturePane 适配为 picker.PaneCapturer，供预览分栏抓屏。
// target 是任意 tmux 目标串（"sess" 或 "sess:3"），原样透传。
//
// window 含多个 pane 时逐个抓取并纵向拼接（pane 间加 dim 分隔线）——
// capture-pane 只抓目标 window 的活动 pane，不拼接的话其余 pane 全部不可见。
type tmuxCapturer struct {
	tmux tmux.Tmux
}

func (c *tmuxCapturer) Capture(target string) (string, error) {
	if c.tmux == nil {
		return "", nil
	}
	indexes, err := c.tmux.ListWindowPanes(target)
	if err != nil || len(indexes) <= 1 {
		// 单 pane / 枚举失败：退回原行为，只抓目标本身（即活动 pane）
		return c.tmux.CapturePane(target)
	}
	parts := make([]string, 0, len(indexes)*2)
	for _, idx := range indexes {
		content, err := c.tmux.CapturePane(fmt.Sprintf("%s.%d", target, idx))
		if err != nil {
			continue
		}
		if len(parts) > 0 {
			// dim 分隔线标出 pane 边界；宽度截断交给 renderPreview
			parts = append(parts, fmt.Sprintf("\x1b[0m\x1b[2m── pane %d ──────────────────────────\x1b[0m", idx))
		}
		parts = append(parts, trimTrailingBlankLines(content))
	}
	if len(parts) == 0 {
		return c.tmux.CapturePane(target)
	}
	return strings.Join(parts, "\n"), nil
}

// trimTrailingBlankLines 裁掉尾部视觉为空的行。capture-pane 按整个 pane 高度
// 返回，内容贴顶的 pane 尾部是成片空行，不裁掉的话分隔线会被推到很远的下方。
func trimTrailingBlankLines(content string) string {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// tmuxKiller 把 tmux.KillSession 适配为 picker.Killer，便于 ctrl+d 直接 kill。
// kill 后顺手 ack 一下 attention，避免幽灵 flag。
type tmuxKiller struct {
	tmux      tmux.Tmux
	attention *attention.Store
}

func (k *tmuxKiller) Kill(name string) error {
	if k.tmux == nil {
		return nil
	}
	if _, err := k.tmux.KillSession(name); err != nil {
		return err
	}
	if k.attention != nil {
		_ = k.attention.Ack(name)
	}
	return nil
}

func windowAgentKey(session string, index int) string { return fmt.Sprintf("%s:%d", session, index) }

type agentSnapshot struct{ sessions, windows map[string]picker.AgentBadge }

func processParents() map[int]int {
	result := map[int]int{}
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		var pid, parent int
		if _, err := fmt.Sscanf(line, "%d %d", &pid, &parent); err == nil {
			result[pid] = parent
		}
	}
	return result
}
func isDescendant(pid, pane int, parents map[int]int) bool {
	seen := map[int]bool{}
	for pid > 0 && !seen[pid] {
		if pid == pane {
			return true
		}
		seen[pid] = true
		pid = parents[pid]
	}
	return false
}
