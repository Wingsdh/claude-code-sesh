package picker

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Wingsdh/cc-sesh/v2/icon"
	"github.com/Wingsdh/cc-sesh/v2/model"
)

type sessionItem struct {
	session    model.SeshSession
	name       string
	searchName string
	src        string
	decoration Decoration
}

// sessionItems 是 picker 的候选列表；模糊匹配只看 searchName（见 match.go）。
type sessionItems []sessionItem

type filteredItem struct {
	item           sessionItem
	matchedIndexes []int
	// windowMatches 是本轮搜索在该 session 的 window 名上命中的结果（无搜索时为空）。
	windowMatches []windowMatch
}

// windowMatch 是某个 session 下被搜索命中的 window，连同它名字里的命中下标。
type windowMatch struct {
	window         WindowItem
	matchedIndexes []int
}

type rowKind int

const (
	rowSession rowKind = iota
	rowWindow
)

// visibleRow 是渲染与光标的唯一依据：把「session 行 + 展开出来的 window 行」
// 拍平成一维序列，光标、滚动、回车全部以它为单位，而不再是 m.filtered 的下标。
type visibleRow struct {
	kind       rowKind
	sessionIdx int // 指向 m.filtered
	// 以下两项仅 rowWindow 有效
	window         WindowItem
	matchedIndexes []int // window 名的命中高亮下标
}

// WindowItem 是 picker 渲染 window 行所需的最小信息，与 tmux 具体实现解耦
// （同 Decoration 的做法）——picker 不知道这些数据是怎么来的，只按字段渲染。
type WindowItem struct {
	SessionName string
	Index       int
	Name        string
	Active      bool
}

// FetchResult 是取数回调的一次性返回体。
// 之所以收成一个结构体而不是继续加返回值：window 清单只在 all/tmux 模式有意义，
// 用具名字段比第 4 个位置参数更难传错。
type FetchResult struct {
	Sessions  model.SeshSessions
	Decorator Decorator
	// Windows 是全部 session 的 window 清单；非 all/tmux 模式为 nil。
	Windows []WindowItem
}

// FetchFunc 在 Init 与 mode 切换时被调用。mode 由 picker 内部按 ctrl+a/t/g/x/f 切换；
// 调用方根据 mode 决定数据源（all/tmux/config/zoxide/find）。
type FetchFunc func(mode string) (FetchResult, error)

// 五种 fetch mode 常量。
const (
	ModeAll    = "all"
	ModeTmux   = "tmux"
	ModeConfig = "config"
	ModeZoxide = "zoxide"
	ModeFind   = "find"
)

type sessionsLoadedMsg struct {
	sessions  model.SeshSessions
	decorator Decorator
	windows   []WindowItem
	err       error
}

type Model struct {
	allItems       sessionItems
	filtered       []filteredItem
	filterInput    textinput.Model
	cursor         int
	offset         int
	width          int
	height         int
	chosen         string
	quit           bool
	showIcons      bool
	separatorAware bool
	focusCmd       tea.Cmd
	loading        bool
	fetchFunc      FetchFunc
	loadErr        error
	decorator      Decorator
	dismisser      Dismisser
	killer         Killer
	now            func() time.Time
	mode           string // 当前 fetch mode：all/tmux/config/zoxide/find

	// windows 是本轮取数带回的全量 window 清单（非 all/tmux 模式为 nil）；
	// windowsBySession 是它按 session 名分组、组内按 Index 升序后的索引。
	windows          []WindowItem
	windowsBySession map[string][]WindowItem

	// rows 是拍平后的可见行序列，cursor / offset 都以它为单位。
	rows []visibleRow
	// expanded 是用户用 →/← 手动维护的展开集合，会落盘；
	// tempExpanded 是搜索命中 window 时的临时展开，NEVER 落盘、清空搜索即失效。
	// 两者刻意分开：混成一个集合会让搜索把用户的手动展开态污染进磁盘。
	expanded     map[string]struct{}
	tempExpanded map[string]struct{}
	expandStore  ExpandStore

	preview  previewState
	capturer PaneCapturer
}

// srcIcon 返回 sesh 原本的来源 icon + ANSI 颜色。
func srcIcon(src string) (string, color.Color) {
	if g, ok := icon.Glyphs[src]; ok {
		var ansi int
		switch {
		case g.ColorCode >= 90 && g.ColorCode <= 97:
			ansi = g.ColorCode - 82
		case g.ColorCode >= 30 && g.ColorCode <= 37:
			ansi = g.ColorCode - 30
		default:
			ansi = g.ColorCode
		}
		return g.Icon + " ", lipgloss.ANSIColor(ansi)
	}
	return "? ", lipgloss.ANSIColor(8)
}

var separatorReplacer = strings.NewReplacer("-", " ", "_", " ", "/", " ", "\\", " ")

func normalizeSeparators(s string) string {
	return separatorReplacer.Replace(s)
}

func buildItems(sessions model.SeshSessions, dec Decorator, separatorAware bool) sessionItems {
	if dec == nil {
		dec = NoDecoration{}
	}
	items := make(sessionItems, 0, len(sessions.OrderedIndex))
	for _, key := range sessions.OrderedIndex {
		s := sessions.Directory[key]
		searchName := s.Name
		if separatorAware {
			searchName = normalizeSeparators(s.Name)
		}
		items = append(items, sessionItem{
			session:    s,
			name:       s.Name,
			searchName: searchName,
			src:        s.Src,
			decoration: dec.Decorate(s),
		})
	}
	return items
}

func New(fetchFunc FetchFunc, dec Decorator, dis Dismisser, kil Killer, showIcons, separatorAware bool, prompt, placeholder string) Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = prompt

	if dec == nil {
		dec = NoDecoration{}
	}

	m := Model{
		filterInput:    ti,
		showIcons:      showIcons,
		separatorAware: separatorAware,
		loading:        true,
		fetchFunc:      fetchFunc,
		decorator:      dec,
		dismisser:      dis,
		killer:         kil,
		now:            time.Now,
		mode:           ModeAll,
		expanded:       map[string]struct{}{},
		tempExpanded:   map[string]struct{}{},
	}
	m.focusCmd = m.filterInput.Focus()
	return m
}

// WithExpandStore 注入折叠记忆存储并立刻载入手动展开集合。
// 做成链式 setter（照 attention.Store.WithClock 先例）而不是加 New() 参数，
// 既有 10 处 New() 调用点因此零改动。
//
// 传 nil → 退化为纯内存展开状态：不读盘、不写盘。
func (m Model) WithExpandStore(store ExpandStore) Model {
	m.expandStore = store
	m.expanded = map[string]struct{}{}
	if store != nil {
		// 拷一份而不是直接持有 store 返回的 map：后续的展开/折起会就地改这个集合，
		// 直接持有会把 picker 的内存状态渗回存储实现内部。
		for name := range store.LoadExpanded() {
			m.expanded[name] = struct{}{}
		}
	}
	return m
}

// WithCapturer 注入抓屏能力。传 nil → picker 不发起任何抓屏，预览区显示无目标说明，
// 既有未注入 Capturer 的调用方行为完全不变。
func (m Model) WithCapturer(c PaneCapturer) Model {
	m.capturer = c
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.focusCmd, m.fetchSessions())
}

func (m Model) fetchSessions() tea.Cmd {
	mode := m.mode
	return func() tea.Msg {
		res, err := m.fetchFunc(mode)
		return sessionsLoadedMsg{
			sessions:  res.Sessions,
			decorator: res.Decorator,
			windows:   res.Windows,
			err:       err,
		}
	}
}

// switchMode 切换数据源并触发异步 fetch。
func (m *Model) switchMode(mode string) tea.Cmd {
	if m.mode == mode {
		return nil
	}
	m.mode = mode
	m.loading = true
	m.cursor = 0
	m.offset = 0
	m.filterInput.SetValue("")
	// 搜索临时展开只属于上一轮搜索，切数据源时一并清掉；
	// expanded 是用户的手动记忆，切回 all/tmux 时还要按它呈现，绝不清。
	m.tempExpanded = map[string]struct{}{}
	return m.fetchSessions()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, tea.Quit
		}
		m.loading = false
		if msg.decorator != nil {
			m.decorator = msg.decorator
		}
		m.windows = msg.windows
		m.windowsBySession = groupWindowsBySession(msg.windows)
		m.allItems = buildItems(msg.sessions, m.decorator, m.separatorAware)
		m.applyFilter()
		return m, nil

	case previewTickMsg:
		// 六步第 4 步：定时到达先校验序号。光标已经移走 → 直接丢弃，连抓屏都不发起。
		if msg.seq != m.preview.seq || msg.target != m.preview.target {
			return m, nil
		}
		if m.capturer == nil {
			return m, nil
		}
		// 六步第 5 步：异步抓屏
		return m, m.capturePreview(msg.seq, msg.target)

	case previewResultMsg:
		// 六步第 6 步：结果到达再校验序号与目标串。两道校验缺一不可——
		// 只校验序号会让「离开又回到同一目标」的旧结果蒙混过关，
		// 只校验目标串则挡不住同一目标的连续重取。
		if msg.seq != m.preview.seq || msg.target != m.preview.target {
			return m, nil
		}
		m.preview.loading = false
		if msg.err != nil {
			m.preview.err = msg.err.Error()
			m.preview.content = ""
		} else {
			m.preview.content = msg.content
			m.preview.err = ""
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filterInput.SetWidth(m.contentWidth() - 4)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if m.loading {
				return m, nil
			}
			if row, ok := m.rowAt(m.cursor); ok {
				name := m.filtered[row.sessionIdx].item.name
				m.chosen = name
				if row.kind == rowWindow {
					// 会话名含 ':' 时 windowTarget 会拒绝拼接：宁可少切一层，
					// 也不要拼出一个 tmux 会解析成别的东西的目标串。
					if target, ok := windowTarget(name, row.window.Index); ok {
						m.chosen = target
					}
				}
			}
			return m, tea.Quit

		case "esc", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "up", "ctrl+k", "shift+tab":
			m.cursorUp(1)
			return m, m.retargetPreview(false)

		case "down", "ctrl+j", "tab":
			m.cursorDown(1)
			return m, m.retargetPreview(false)

		case "right":
			m.expandCurrent()
			return m, m.retargetPreview(false)

		case "left":
			m.collapseCurrent()
			return m, m.retargetPreview(false)

		case "ctrl+r":
			// 强制重取当前快照：无条件走完六步，旧结果因序号自增自然作废。
			return m, m.retargetPreview(true)

		case "ctrl+u":
			m.cursorUp(m.visibleCount() / 2)
			return m, m.retargetPreview(false)

		case "pgdown":
			m.cursorDown(m.visibleCount() / 2)
			return m, m.retargetPreview(false)

		case "pgup":
			m.cursorUp(m.visibleCount() / 2)
			return m, m.retargetPreview(false)

		case "ctrl+a":
			return m, m.switchMode(ModeAll)

		case "ctrl+t":
			return m, m.switchMode(ModeTmux)

		case "ctrl+g":
			return m, m.switchMode(ModeConfig)

		case "ctrl+x":
			return m, m.switchMode(ModeZoxide)

		case "ctrl+f":
			return m, m.switchMode(ModeFind)

		case "ctrl+d":
			// window 行不支持 kill：一行都不做，直接返回。
			if row, ok := m.rowAt(m.cursor); ok && row.kind == rowWindow {
				return m, nil
			}
			// kill 当前 cursor 所指 tmux session（与 fzf-tmux 习惯一致）。
			// session 不存在后 attention 也会被自然 GC，无需额外清理。
			m.killCurrent()
			return m, nil

		case "alt+d":
			// window 行没有自己的 attention 标记，dismiss 无从谈起。
			if row, ok := m.rowAt(m.cursor); ok && row.kind == rowWindow {
				return m, nil
			}
			// 手动 dismiss 当前 attention 行。alt+d 避开和搜索字符 'd' 冲突。
			m.dismissCurrent()
			return m, nil
		}
	}

	prevValue := m.filterInput.Value()
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)

	if m.filterInput.Value() != prevValue {
		if !m.loading {
			m.applyFilter()
		}
		m.cursor = 0
		m.offset = 0
		return m, tea.Batch(cmd, m.retargetPreview(false))
	}

	return m, cmd
}

// killCurrent kill 当前 cursor 所指的 tmux session。仅对 src=tmux 有效；
// 其他类型（zoxide / config / tmuxinator template）没真实 session 可 kill，no-op。
func (m *Model) killCurrent() {
	if m.killer == nil {
		return
	}
	row, ok := m.rowAt(m.cursor)
	if !ok || row.kind != rowSession {
		return
	}
	cur := m.filtered[row.sessionIdx].item
	if cur.session.Src != "tmux" {
		return
	}
	if err := m.killer.Kill(cur.name); err != nil {
		return
	}
	// 从 allItems 移除被 kill 的项，让列表立即同步
	newItems := make(sessionItems, 0, len(m.allItems))
	for _, it := range m.allItems {
		if it.name == cur.name && it.session.Src == "tmux" {
			continue
		}
		newItems = append(newItems, it)
	}
	m.allItems = newItems
	// applyFilter 会重算可见行并夹紧光标——被 kill 的 session 展开出来的 window 行
	// 必须随之消失，否则会留下指向不存在 session 下标的孤儿行。
	m.applyFilter()
}

// dismissCurrent 在 cursor 落在 attention 行时清除该行的 flag，并重新装饰所有项。
func (m *Model) dismissCurrent() {
	if m.dismisser == nil {
		return
	}
	row, ok := m.rowAt(m.cursor)
	if !ok || row.kind != rowSession {
		return
	}
	cur := m.filtered[row.sessionIdx].item
	if !cur.decoration.Attention.Triggered {
		return
	}
	if err := m.dismisser.Dismiss(cur.name); err != nil {
		return
	}
	// 重新装饰：对所有 allItems 再调一次 decorator.Decorate
	for i := range m.allItems {
		m.allItems[i].decoration = m.decorator.Decorate(m.allItems[i].session)
	}
	m.applyFilter()
}

func (m *Model) applyFilter() {
	pattern := m.filterInput.Value()

	if pattern == "" {
		m.filtered = make([]filteredItem, len(m.allItems))
		for i, item := range m.allItems {
			m.filtered[i] = filteredItem{item: item}
		}
		// 没有搜索就没有临时展开：回到 expanded 决定的形态，expanded 一字不动。
		m.tempExpanded = map[string]struct{}{}
	} else {
		searchPat := pattern
		if m.separatorAware {
			searchPat = normalizeSeparators(pattern)
		}

		matches := fuzzyFind(searchPat, m.allItems.searchNames())
		m.filtered = make([]filteredItem, 0, len(matches))
		posByName := make(map[string]int, len(matches))
		for _, match := range matches {
			it := m.allItems[match.index]
			posByName[it.name] = len(m.filtered)
			m.filtered = append(m.filtered, filteredItem{
				item:           it,
				matchedIndexes: match.positions,
			})
		}

		// 搜索范围扩到 window 名：连未展开 session 的 window 也要参与匹配，
		// 否则用户得先猜到 window 在哪个 session 里才搜得到它。
		temp := make(map[string]struct{})
		for _, it := range m.allItems {
			wins := m.windowsBySession[it.name]
			if len(wins) == 0 {
				continue
			}
			wm := matchWindowNames(searchPat, wins, m.separatorAware)
			if len(wm) == 0 {
				continue
			}
			temp[it.name] = struct{}{}
			if pos, ok := posByName[it.name]; ok {
				m.filtered[pos].windowMatches = wm
				continue
			}
			// 会话名本身没命中、只是它下面的 window 命中了 → 追加到末尾，
			// 不抢会话名直接命中的项的位置。
			posByName[it.name] = len(m.filtered)
			m.filtered = append(m.filtered, filteredItem{item: it, windowMatches: wm})
		}
		m.tempExpanded = temp
	}

	// attention 项稳定排序到前面（只作用于 session 层，window 行永不参与顶层排序）
	sort.SliceStable(m.filtered, func(i, j int) bool {
		ai := m.filtered[i].item.decoration.Attention.Triggered
		aj := m.filtered[j].item.decoration.Attention.Triggered
		if ai != aj {
			return ai
		}
		return false
	})

	m.rebuildRows()
}

// matchWindowNames 对一个 session 的全部 window 名做 fuzzy，返回命中项（按 Index 升序）。
func matchWindowNames(searchPat string, wins []WindowItem, separatorAware bool) []windowMatch {
	names := make([]string, len(wins))
	for i, w := range wins {
		n := w.Name
		if separatorAware {
			n = normalizeSeparators(n)
		}
		names[i] = n
	}
	found := fuzzyFind(searchPat, names)
	if len(found) == 0 {
		return nil
	}
	out := make([]windowMatch, 0, len(found))
	for _, f := range found {
		out = append(out, windowMatch{
			window:         wins[f.index],
			matchedIndexes: f.positions,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].window.Index < out[j].window.Index })
	return out
}

// groupWindowsBySession 把全量 window 清单按所属 session 分组，组内按 Index 升序。
// 排序放在这里做一次，后续 buildVisibleRows 就不用每帧重排。
func groupWindowsBySession(windows []WindowItem) map[string][]WindowItem {
	out := make(map[string][]WindowItem)
	for _, w := range windows {
		out[w.SessionName] = append(out[w.SessionName], w)
	}
	for name := range out {
		ws := out[name]
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].Index < ws[j].Index })
		out[name] = ws
	}
	return out
}

// buildVisibleRows 是可见行的唯一真源：三份输入决定全部可见行，无副作用。
//
// 每个 session 行之后按下列优先级追加它自己的 window 行：
//  1. 该 session 在临时展开集合里且本轮有搜索命中 → 只出命中的那些 window 行
//  2. 否则在手动展开集合里 → 出它全部 window 行
//  3. 否则不出
//
// 同时落在两个集合里走 1：搜索时用户想看的是「命中了什么」，不是整个列表。
func buildVisibleRows(
	filtered []filteredItem,
	expanded map[string]struct{},
	temp map[string]struct{},
	windowsBySession map[string][]WindowItem,
) []visibleRow {
	rows := make([]visibleRow, 0, len(filtered))
	for i, fi := range filtered {
		rows = append(rows, visibleRow{kind: rowSession, sessionIdx: i})
		name := fi.item.name

		if _, inTemp := temp[name]; inTemp && len(fi.windowMatches) > 0 {
			matched := append([]windowMatch(nil), fi.windowMatches...)
			sort.SliceStable(matched, func(a, b int) bool {
				return matched[a].window.Index < matched[b].window.Index
			})
			for _, wm := range matched {
				rows = append(rows, visibleRow{
					kind:           rowWindow,
					sessionIdx:     i,
					window:         wm.window,
					matchedIndexes: wm.matchedIndexes,
				})
			}
			continue
		}

		if _, inExpanded := expanded[name]; inExpanded {
			wins := append([]WindowItem(nil), windowsBySession[name]...)
			sort.SliceStable(wins, func(a, b int) bool { return wins[a].Index < wins[b].Index })
			for _, w := range wins {
				rows = append(rows, visibleRow{kind: rowWindow, sessionIdx: i, window: w})
			}
		}
	}
	return rows
}

// rebuildRows 重算可见行并夹紧光标。
// 取数完成、搜索词变化、展开集合变化、kill session、切换数据源之后都必须走它——
// 行数暴增或骤减而不夹紧，光标就会指向不存在的行。
func (m *Model) rebuildRows() {
	m.rows = buildVisibleRows(m.filtered, m.expanded, m.tempExpanded, m.windowsBySession)
	m.clampCursor()
}

func (m Model) rowAt(i int) (visibleRow, bool) {
	if i < 0 || i >= len(m.rows) {
		return visibleRow{}, false
	}
	return m.rows[i], true
}

// isExpandable 判定 m.filtered[sessionIdx] 能否展开：
// 必须是真实 tmux session，且它名下至少有一个 window。
// zoxide / config / tmuxinator / find 来源的项没有真实 session，永远不可展开。
func (m Model) isExpandable(sessionIdx int) bool {
	if sessionIdx < 0 || sessionIdx >= len(m.filtered) {
		return false
	}
	it := m.filtered[sessionIdx].item
	if it.src != "tmux" {
		return false
	}
	return len(m.windowsBySession[it.name]) >= 1
}

// clampCursor 把 cursor 与 offset 夹回合法区间，保证
// [offset, offset+visibleCount()) 始终包含 cursor。
func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.offset > m.cursor {
		m.offset = m.cursor
	}
	if visible := m.visibleCount(); m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// expandCurrent 处理 → 键：只对「光标停在可展开的 session 行」生效。
// 不可展开、或光标已在 window 行 → 静默无反应，且不写盘。
func (m *Model) expandCurrent() {
	row, ok := m.rowAt(m.cursor)
	if !ok || row.kind != rowSession || !m.isExpandable(row.sessionIdx) {
		return
	}
	if m.expanded == nil {
		m.expanded = map[string]struct{}{}
	}
	m.expanded[m.filtered[row.sessionIdx].item.name] = struct{}{}
	m.persistExpanded()
	m.rebuildRows()
}

// collapseCurrent 处理 ← 键：把光标所属 session 从两个展开集合里移除。
// 光标在 window 行时把光标收回该 session 行；本来就折叠则无反应。
//
// 只有「确实从手动展开集合里移除了」才写盘——搜索临时展开是本轮搜索的产物，
// 把它的折起写进磁盘会让用户下次打开发现自己从没手动折过的 session 被折了。
func (m *Model) collapseCurrent() {
	row, ok := m.rowAt(m.cursor)
	if !ok {
		return
	}
	idx := row.sessionIdx
	if idx < 0 || idx >= len(m.filtered) {
		return
	}
	name := m.filtered[idx].item.name

	_, wasExpanded := m.expanded[name]
	_, wasTemp := m.tempExpanded[name]
	if !wasExpanded && !wasTemp {
		return
	}

	delete(m.expanded, name)
	delete(m.tempExpanded, name)
	if wasExpanded {
		m.persistExpanded()
	}

	m.rows = buildVisibleRows(m.filtered, m.expanded, m.tempExpanded, m.windowsBySession)
	if row.kind == rowWindow {
		// 折起后原来的 window 行已经不存在了，光标必须落到它所属的 session 行上
		for i, r := range m.rows {
			if r.kind == rowSession && r.sessionIdx == idx {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

// persistExpanded 把当前手动展开集合写盘。写失败只影响下次记忆，不打断 picker。
func (m *Model) persistExpanded() {
	if m.expandStore == nil {
		return
	}
	names := make([]string, 0, len(m.expanded))
	for name := range m.expanded {
		names = append(names, name)
	}
	sort.Strings(names)
	_ = m.expandStore.SaveExpanded(names)
}

// windowTarget 拼出 tmux 的 window 目标串。
// 会话名本身含 ':' 时无法拼出无歧义的目标 → ok=false，调用方退化为只用会话名。
func windowTarget(session string, index int) (string, bool) {
	if strings.Contains(session, ":") {
		return "", false
	}
	return fmt.Sprintf("%s:%d", session, index), true
}

func (m *Model) cursorUp(n int) {
	m.cursor -= n
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
}

func (m *Model) cursorDown(n int) {
	m.cursor += n
	max := len(m.rows) - 1
	if max < 0 {
		max = 0
	}
	if m.cursor > max {
		m.cursor = max
	}
	visible := m.visibleCount()
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m Model) visibleCount() int {
	// chrome: filter(1) + hotkey(1) + table-top(1) + header(1) + 2 blank + section overhead(~5)
	chrome := 12
	if !m.showSessionStateTable() {
		chrome -= 2 // 同时隐藏 table-top 和列头时让出两行
	}
	// 列表行数随终端高度伸缩，不设固定上限——旧的 15 行 cap 是 70% 高
	// popup 时代的遗产，高 popup 里会让列表下方大片留白。
	available := m.height - chrome
	if available < 1 {
		available = 3
	}
	return available
}

// showSessionStateTable 决定是否渲染 Claude Code 状态表（table-top 横线 + ATTN/IDLE/RUN/WAIT 列头）。
// 仅在以下两个条件都满足时渲染：
//  1. 当前 mode 是 all 或 tmux（其他 mode 的列值始终为空）
//  2. 当前至少有一个 session 有 Claude Code 在运行（Live 非空 或 Attention 触发）
func (m Model) showSessionStateTable() bool {
	if m.mode != ModeAll && m.mode != ModeTmux {
		return false
	}
	for _, it := range m.allItems {
		if !it.decoration.Live.IsEmpty() || it.decoration.Attention.Triggered {
			return true
		}
	}
	return false
}

func (m Model) contentWidth() int {
	w := m.width
	if w < 30 {
		w = 40
	}
	if m.showPreview() {
		// 分栏时列表按比例分宽：占 listRatio%，但不低于基准 60 列。
		// 固定 60 列会让宽终端下多余宽度全被预览吃掉，列表显得越来越挤；
		// 按比例分配后列表/预览观感稳定在约 4:6，与终端多宽无关。
		if p := m.width * listRatio / 100; p > listBaseWidth {
			return p
		}
		return listBaseWidth
	}
	if w > listBaseWidth {
		w = listBaseWidth
	}
	return w
}

// 预览分栏的布局常量。previewMinTotal 是「终端够不够宽渲染预览」的唯一阈值。
const (
	previewGap      = 2                                            // 列表区与预览区的分隔间距
	listBaseWidth   = 60                                           // 列表区基准宽：窄终端的上限，分栏时的下限
	listRatio       = 40                                           // 分栏时列表区占终端总宽的百分比
	previewMinWidth = 40                                           // 预览区下限
	previewMinTotal = listBaseWidth + previewGap + previewMinWidth // 102：显示阈值
)

// showPreview 每帧现算，NEVER 缓存进 Model——
// 缓存会让拉宽终端后预览回不来（既有 tea.WindowSizeMsg 就是唯一的恢复通道）。
func (m Model) showPreview() bool {
	return m.width >= previewMinTotal
}

// previewWidth 是预览块的宽度。width >= 102 时 contentWidth() 不低于 60 且
// 不超过 width 的 listRatio%，故 previewWidth() >= previewMinWidth 恒成立。
func (m Model) previewWidth() int {
	return m.width - m.contentWidth() - previewGap
}

// 颜色调色：尽量用 ANSI 数字，确保跨终端一致。
var (
	colorCursor   = lipgloss.ANSIColor(2)   // green
	colorAttn     = lipgloss.ANSIColor(208) // 256-color 橙；ATTN 列圆点
	colorBusy     = lipgloss.ANSIColor(12)  // bright blue
	colorSubagent = lipgloss.ANSIColor(11)  // bright yellow
	colorNeeding  = lipgloss.ANSIColor(11)  // bright yellow（WAIT 列）
	colorRun      = lipgloss.ANSIColor(14)  // bright cyan（RUN 列）
	colorIdle     = lipgloss.ANSIColor(8)   // bright black / dim
	colorMatch    = lipgloss.ANSIColor(1)   // red
	colorTail     = lipgloss.ANSIColor(8)
	colorHeader   = lipgloss.ANSIColor(8)
)

func (m Model) View() tea.View {
	list := m.renderList()
	if !m.showPreview() {
		return tea.NewView(list)
	}
	// 列表块补齐到 contentWidth()，否则短行会让预览块的左边缘参差不齐
	listBlock := lipgloss.NewStyle().Width(m.contentWidth()).Render(list)
	previewBlock := m.renderPreview(m.previewWidth(), m.previewHeight())
	gap := lipgloss.NewStyle().Width(previewGap).Render("")
	return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top, listBlock, gap, previewBlock))
}

// previewHeight 是预览块的高度预算。预览块与整个列表块并排（JoinHorizontal Top），
// 从视图顶部一路可用到底，因此按终端高度给、不绑列表的 visibleCount()——
// 后者要扣掉列表上方的 chrome 行数，预览没有这份开销。留 1 行余量防终端滚动。
func (m Model) previewHeight() int {
	return max(3, m.height-1)
}

// fitRow 把一条列表行截到 contentWidth 内（ANSI 感知，截断时以 … 结尾）。
// 不截断的话，超宽行（长 zoxide 路径实测踩中）会被列表块的 Width(contentWidth)
// 折成多个物理行，把列表块撑得比终端高，引发整屏滚动错位。
func (m Model) fitRow(line string) string {
	return ansi.Truncate(line, m.contentWidth(), "…")
}

// renderList 渲染左侧列表块。预览是否显示不影响它的任何一行——
// visibleCount() 也不受预览影响，窄终端下的可见条数与改动前一致。
func (m Model) renderList() string {
	var b strings.Builder

	b.WriteString("  " + m.filterInput.View())
	b.WriteString("\n")
	b.WriteString(renderHotkeyHeader(m.mode))
	b.WriteString("\n")
	if m.showSessionStateTable() {
		b.WriteString(renderTableTop(m.showIcons, m.contentWidth()))
		b.WriteString("\n")
		b.WriteString(renderColumnHeaders(m.showIcons))
		b.WriteString("\n")
	}

	visible := m.visibleCount()

	if m.loading {
		loadingStyle := lipgloss.NewStyle().Faint(true)
		b.WriteString(loadingStyle.Render("  Loading sessions..."))
		b.WriteString("\n")
		for i := 1; i < visible; i++ {
			b.WriteString("\n")
		}
	} else {
		end := m.offset + visible
		if end > len(m.rows) {
			end = len(m.rows)
		}

		// 计算 needs-you 数；> 0 时第一行渲染分组标题（如果在可视范围内）。
		needsCount := 0
		for _, fi := range m.filtered {
			if fi.item.decoration.Attention.Triggered {
				needsCount++
			}
		}

		linesPrinted := 0
		printedHeader := needsCount == 0 // 没 attention 时不显示
		printedDivider := false

		for i := m.offset; i < end; i++ {
			row := m.rows[i]
			fi := m.filtered[row.sessionIdx]

			// window 行不参与 attention 分组：它属于上面那条 session 行的展开内容，
			// 在它前面插 header / 分割线会把一个 session 的行拦腰劈开。
			if row.kind == rowWindow {
				b.WriteString(m.fitRow(m.renderWindowRow(row, i == m.cursor)))
				b.WriteString("\n")
				linesPrinted++
				continue
			}

			// 在第一条 attention 行前插 "needs you" header
			if !printedHeader && fi.item.decoration.Attention.Triggered {
				b.WriteString(renderHeader(fmt.Sprintf("needs you (%d)", needsCount)))
				b.WriteString("\n")
				printedHeader = true
				linesPrinted++
			}
			// 在第一条非 attention 行前插分割线（前提是 attention 区有内容）
			if needsCount > 0 && !printedDivider && !fi.item.decoration.Attention.Triggered {
				b.WriteString(renderDivider())
				b.WriteString("\n")
				printedDivider = true
				linesPrinted++
			}

			b.WriteString(m.fitRow(m.renderRow(fi, i == m.cursor)))
			b.WriteString("\n")
			linesPrinted++
		}

		for i := linesPrinted; i < visible; i++ {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderHeader(label string) string {
	return lipgloss.NewStyle().Foreground(colorHeader).Faint(true).Render("  ─── " + label + " ──")
}

// renderHotkeyHeader 在搜索框下方显示模式切换的 hotkey 提示，当前 mode 高亮。
func renderHotkeyHeader(mode string) string {
	dim := lipgloss.NewStyle().Foreground(colorHeader).Faint(true)
	hi := lipgloss.NewStyle().Foreground(colorCursor).Bold(true)
	pick := func(name, label string) string {
		if mode == name {
			return hi.Render(label)
		}
		return dim.Render(label)
	}
	parts := []string{
		pick(ModeAll, "^a all"),
		pick(ModeTmux, "^t tmux"),
		pick(ModeConfig, "^g configs"),
		pick(ModeZoxide, "^x zoxide"),
		pick(ModeFind, "^f find"),
		dim.Render("^d kill"),
	}
	return "  " + strings.Join(parts, dim.Render("  "))
}

func renderDivider() string {
	return lipgloss.NewStyle().Foreground(colorHeader).Faint(true).Render("  ───────────────")
}

// 表格列宽常量。每行的 ATTN/IDLE/RUN/WAIT 4 列，前 3 列宽 5（含右侧分隔空格），
// 最后一列宽 4。Header 字符串与行内 cell 必须使用相同宽度，确保竖向对齐。
const (
	colCellWidth     = 5
	colLastCellWidth = 4
	colsTotalWidth   = colCellWidth*3 + colLastCellWidth // 19
)

// 徽章区留白与 window 行缩进。**三处渲染（列头 / 数据行 / 表格横线）必须读同一组常量**——
// 任何一处写成 2 / 3 的字面量，改动时都会让竖向对齐悄悄散架。
const (
	badgeLeftPad     = 2 // 徽章区左内缩：ATTN 首字符与左侧内容之间的空格数
	badgeRightGap    = 3 // 徽章区右间隔：WAIT 列末与 session 名之间（原为 1）
	windowIndentStep = 2 // window 行在 session 名字列起点之上再缩进一级
)

// window 行的前缀与活动标记。
const (
	windowRowPrefix  = "└ "
	windowActiveMark = " *"
)

// badgeBlockStart 是徽章区（含左内缩）之前占掉的列数：光标列 + 可选的来源图标列。
// 列头与表格横线都从这里起算，保证三处对齐。
func badgeBlockStart(showIcons bool) int {
	leftPad := 2 // 光标列
	if showIcons {
		leftPad += 2 // 来源图标列
	}
	return leftPad + badgeLeftPad
}

// 表格列布局：每个数字 cell 4 字符宽（居中对齐），后跟 1 字符分隔（最后列无尾分隔）。
const colNumWidth = 4

// renderTableTop 输出表格上方的横线，把表格区与 hotkey 区视觉分隔开。
// 横线长度 = contentWidth - leftPad，覆盖整张表格（含 name 区）。
func renderTableTop(showIcons bool, contentWidth int) string {
	leftPad := badgeBlockStart(showIcons)
	lineLen := contentWidth - leftPad
	if lineLen < colsTotalWidth {
		lineLen = colsTotalWidth
	}
	style := lipgloss.NewStyle().Foreground(colorHeader).Faint(true)
	return strings.Repeat(" ", leftPad) + style.Render(strings.Repeat("─", lineLen))
}

// renderColumnHeaders 输出表格列标题，与每行的 4 列徽章列竖向对齐。
//
// 左侧 padding = cursor(2) + src_icon(showIcons ? 2 : 0)，刚好对齐到行内 ATTN 列起点。
// 标题用 bold 默认前景色，比 dim 更醒目。
func renderColumnHeaders(showIcons bool) string {
	leftPad := badgeBlockStart(showIcons)
	style := lipgloss.NewStyle().Bold(true)
	cell := func(label string, last bool) string {
		s := style.Width(colNumWidth).Align(lipgloss.Center).Render(label)
		if !last {
			s += " "
		}
		return s
	}
	return strings.Repeat(" ", leftPad) +
		cell("ATTN", false) +
		cell("IDLE", false) +
		cell("RUN", false) +
		cell("WAIT", true)
}

// renderRowCounts 渲染单行的 4 列徽章数字（19 字符宽）。
//
//   - ATTN：橙底白字 ⚠ 色块（粘性提醒）；未触发为空白
//   - IDLE / RUN / WAIT：右对齐数字。0 dim、非零彩色加粗。
//   - 整行无 Claude（src 非 tmux 等）→ 全列空白对齐
func renderRowCounts(dec Decoration) string {
	// 整体空白：非 tmux session（无 live、无 attention）
	if dec.Live.IsEmpty() && !dec.Attention.Triggered {
		return strings.Repeat(" ", colsTotalWidth)
	}

	// ATTN 列：橙色圆点居中（与 IDLE/RUN/WAIT 同款 cell 形状）
	var attnCell string
	if dec.Attention.Triggered {
		dot := lipgloss.NewStyle().
			Foreground(colorAttn).
			Bold(true).
			Width(colNumWidth).
			Align(lipgloss.Center).
			Render("●")
		attnCell = dot + " "
	} else {
		attnCell = strings.Repeat(" ", colCellWidth)
	}

	// IDLE/RUN/WAIT 列：数字居中对齐 4 宽 + 右侧 1 空格分隔（最后列无尾空格）
	cell := func(n int, clr lipgloss.ANSIColor, last bool) string {
		width := colCellWidth
		if last {
			width = colLastCellWidth
		}
		if dec.Live.IsEmpty() {
			return strings.Repeat(" ", width)
		}
		txt := fmt.Sprintf("%d", n)
		st := lipgloss.NewStyle().Width(colNumWidth).Align(lipgloss.Center)
		if n == 0 {
			st = st.Foreground(colorIdle).Faint(true)
		} else {
			st = st.Foreground(clr).Bold(true)
		}
		styled := st.Render(txt)
		if !last {
			styled += " "
		}
		return styled
	}

	return attnCell +
		cell(dec.Live.Idle(), colorIdle, false) +
		cell(dec.Live.Busy+dec.Live.Subagent, colorRun, false) +
		cell(dec.Live.Needing, colorNeeding, true)
}

func (m Model) renderRow(fi filteredItem, isCursor bool) string {
	item := fi.item
	dec := item.decoration

	// 1. cursor 列（2 字符）
	cursorPrefix := "  "
	if isCursor {
		cursorPrefix = lipgloss.NewStyle().Foreground(colorCursor).Bold(true).Render("> ")
	}

	// 2. src icon 列（2 字符；showIcons=false 时省略）
	var srcCol string
	if m.showIcons {
		icn, clr := srcIcon(item.src)
		srcCol = lipgloss.NewStyle().Foreground(clr).Render(icn)
	}

	// 3. ATTN/IDLE/RUN/WAIT 4 列徽章（19 字符）；隐藏状态表时整列省略，让 name 紧贴 src icon
	var countsCol string
	if m.showSessionStateTable() {
		countsCol = strings.Repeat(" ", badgeLeftPad) + renderRowCounts(dec) + strings.Repeat(" ", badgeRightGap)
	}

	// 4. name 列（fuzzy 高亮）
	nameStyle := lipgloss.NewStyle()
	matchStyle := lipgloss.NewStyle().Foreground(colorMatch).Bold(true)
	name := highlightMatches(item.name, fi.matchedIndexes, matchStyle, nameStyle)

	// 5. tail 列
	tail := m.renderTail(dec)

	body := cursorPrefix + srcCol + countsCol + name
	if tail != "" {
		body += "  " + tail
	}
	return body
}

// nameColStart 是 session 名列相对「光标列之后」的起点列数。
// 状态表隐藏时徽章区整块不存在，名字紧贴来源图标。
func (m Model) nameColStart() int {
	start := 0
	if m.showIcons {
		start += 2
	}
	if m.showSessionStateTable() {
		start += badgeLeftPad + colsTotalWidth + badgeRightGap
	}
	return start
}

// renderWindowRow 渲染一条 window 行：光标列 + 缩进 + "└ 序号: 名字"（活动 window 加标记）。
//
// NEVER 渲染 ATTN/IDLE/RUN/WAIT 任何字符，也 NEVER 用空白 cell 占位对齐——
// 留空占位会与「没有 Claude 的 session 行」撞脸，用户分不清哪行是 window。
func (m Model) renderWindowRow(row visibleRow, isCursor bool) string {
	cursorPrefix := "  "
	if isCursor {
		cursorPrefix = lipgloss.NewStyle().Foreground(colorCursor).Bold(true).Render("> ")
	}

	indent := strings.Repeat(" ", m.nameColStart()+windowIndentStep)

	// 搜索高亮与 session 名同款，走既有 highlightMatches
	nameStyle := lipgloss.NewStyle()
	matchStyle := lipgloss.NewStyle().Foreground(colorMatch).Bold(true)
	name := highlightMatches(row.window.Name, row.matchedIndexes, matchStyle, nameStyle)

	body := cursorPrefix + indent + windowRowPrefix + strconv.Itoa(row.window.Index) + ": " + name
	if row.window.Active {
		body += windowActiveMark
	}
	return body
}

// renderTail 在 attention 行末尾显示「完成多久」提示，便于用户判断紧迫度。
func (m Model) renderTail(dec Decoration) string {
	if !dec.Attention.Triggered {
		return ""
	}
	dur := durationShort(m.now().Sub(dec.Attention.FirstAt))
	return lipgloss.NewStyle().Foreground(colorTail).Faint(true).
		Render(fmt.Sprintf("done %s ago", dur))
}

func durationShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func highlightMatches(s string, indexes []int, matchStyle, normalStyle lipgloss.Style) string {
	if len(indexes) == 0 {
		return normalStyle.Render(s)
	}

	matchSet := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		matchSet[idx] = true
	}

	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		ch := string(r)
		if matchSet[i] {
			result.WriteString(matchStyle.Render(ch))
		} else {
			result.WriteString(normalStyle.Render(ch))
		}
	}
	return result.String()
}

func (m Model) Chosen() string { return m.chosen }
func (m Model) Quit() bool     { return m.quit }
func (m Model) LoadErr() error { return m.loadErr }
func (m Model) Loading() bool  { return m.loading }
