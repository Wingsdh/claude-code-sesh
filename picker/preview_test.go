package picker

// Partition table (ECP + BVA) — step: 03-render-preview — picker 包
//
// 用例编号（tc-aXX）对应 ai-plans/testcases-picker.md §1.5/1.6/1.11/1.12。
// 渲染类断言优先用「结构对齐 / 列位测量」而不是断言常量字面量本身——
// 常量本身等于 2/3 不能证明三处渲染真的共用它，量列位才能抓到「某处改字面量后
// 竖向对齐散架」这种回归（团队要求）。
//
// ECP/BVA 划分表（节选）：
//
//	目标                          等价类                        期望                              来源
//	renderTableTop/Headers/Row    三处渲染                      徽章区起始列位三处相等            tc-a26
//	badgeLeftPad/badgeRightGap    左内缩/右间隔（下边界=2/3）    实测值>=2/>=3 且三处一致          tc-a27/a28
//	renderTableTop 横线长度       正常 contentWidth              = contentWidth-leftPad            tc-a29
//	renderTableTop 横线长度       contentWidth 极小(边界)        不短于 colsTotalWidth(19)         tc-a29
//	showSessionStateTable()       false(边界)                    countsCol 为空串，留白不生效       tc-a30
//	showPreview()                 width=101(下边界外) / 102(下边界内) false / true                 布局约束
//	previewWidth()                width=102(边界)/120/150(换挡)/160/200  40/58/88/94/118（列表40%下限60）  tc-a75
//	visibleCount()                预览显示/隐藏两态               取值相同                          tc-a76
//	取快照六步                    happy path / 4 种丢弃分支      见 Section D                      tc-a32~a39/a73
//	previewTarget                 session行有active/无active(退回最小Index)/window表空/非tmux/window行/空列表/Capturer=nil  见 Section E  行为契约+tc-a74
//	renderPreview 截断             行宽>previewWidth(上边界外)   宽度<=previewWidth 且 ANSI 完整   tc-a68
//	renderPreview 截断             行宽==previewWidth(边界)      不截断                            tc-a71
//	renderPreview 行数             超高度(边界外)                 保留末尾若干行                    tc-a69
//	renderPreview 尾部空行         内容贴顶+成片空尾行            先裁空尾行再保留末尾              抓屏回归
//
// pairwise：window 行缩进公式只有 2 个布尔维度（showIcons × 状态表可见），
// 2×2=4 组合直接穷举（TestRenderWindowRow_IndentMatchesNameColStartPlusStep），
// 未达到 ≥3 参数×≥2取值的强制门槛，未调用 pairwise 脚本。
//
// 关于「真实等 150ms」的测试策略说明：
// previewTickMsg/previewResultMsg 里的 seq 由生产代码内部维护，字段名未在契约里
// 暴露给测试（只有 previewState/previewTarget/retargetPreview/renderPreview 这几个
// 契约点是公开的）。为了不靠猜内部字段名、不硬编码假设的 seq 起始值，本文件里凡是
// 需要精确 seq 值的场景，一律真的调用 retargetPreview 返回的 tea.Cmd（内部是
// tea.Tick(150ms,...)，调用会真的阻塞约 150ms）拿到真实 previewTickMsg，再从这条
// 真实消息里读出 seq/target 用于后续断言或构造「过期」消息——不猜数字，只用真实産出。

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wingsdh/cc-sesh/v2/model"
)

// ---------- 测试基础设施 ----------

// fakeCapturer 是 PaneCapturer 的可记录假实现。
type fakeCapturer struct {
	calls   []string
	content string
	err     error
}

func (f *fakeCapturer) Capture(target string) (string, error) {
	f.calls = append(f.calls, target)
	return f.content, f.err
}

// liveFlagDecorator 让指定 session 名带上 Live 徽章，用来控制 showSessionStateTable()。
type liveFlagDecorator struct{ live map[string]LiveBadge }

func (d liveFlagDecorator) Decorate(s model.SeshSession) Decoration {
	if b, ok := d.live[s.Name]; ok {
		return Decoration{Live: b}
	}
	return Decoration{}
}

// previewTestSessions 是本文件共用的 4 会话夹具：
//
//	s1 proj-a  tmux  → 3 个 window，Index2 是活动 window（正常路径）
//	s2 proj-b  tmux  → 2 个 window（Index 5、2），没有任何活动 window（tc-a74 退回最小 Index）
//	s3 proj-c  tmux  → 0 个 window → 无目标
//	s4 cfgapp  config → 非 tmux → 无目标
func previewTestSessions() model.SeshSessions {
	dir := model.SeshSessionMap{
		"s1": {Name: "proj-a", Src: "tmux"},
		"s2": {Name: "proj-b", Src: "tmux"},
		"s3": {Name: "proj-c", Src: "tmux"},
		"s4": {Name: "cfgapp", Src: "config"},
	}
	return model.SeshSessions{OrderedIndex: []string{"s1", "s2", "s3", "s4"}, Directory: dir}
}

func previewTestWindows() []WindowItem {
	return []WindowItem{
		{SessionName: "proj-a", Index: 1, Name: "claude", Active: false},
		{SessionName: "proj-a", Index: 2, Name: "server", Active: true},
		{SessionName: "proj-a", Index: 3, Name: "shell", Active: false},
		{SessionName: "proj-b", Index: 5, Name: "x", Active: false},
		{SessionName: "proj-b", Index: 2, Name: "y", Active: false},
	}
}

// 全折叠态下 rows == filtered，下标即会话下标（与 previewTestSessions 的声明顺序一致）。
const (
	idxProjA  = 0
	idxProjB  = 1
	idxProjC  = 2
	idxCfgapp = 3
)

// newPreviewModel 通过真实 New()+WithCapturer()+Update(sessionsLoadedMsg) 路径构建
// 已完成取数的 Model，width/height 给足以让 showPreview()==true、visibleCount() 稳定。
func newPreviewModel(t *testing.T, cap PaneCapturer) Model {
	t.Helper()
	sessions := previewTestSessions()
	windows := previewTestWindows()
	fetch := func(string) (FetchResult, error) {
		return FetchResult{Sessions: sessions, Decorator: NoDecoration{}, Windows: windows}, nil
	}
	m := New(fetch, NoDecoration{}, nil, nil, false, false, "> ", "Filter sessions...")
	m = m.WithCapturer(cap)
	result, _ := m.Update(sessionsLoadedMsg{sessions: sessions, decorator: NoDecoration{}, windows: windows})
	m2 := result.(Model)
	m2.width = 120
	m2.height = 30
	return m2
}

// modelWithStateTable 构建一个能精确控制 showSessionStateTable() 输出的 Model，
// 用于 Section A/B 的留白/缩进结构测试。
func modelWithStateTable(t *testing.T, showIcons, tableVisible bool) Model {
	t.Helper()
	sessions := previewTestSessions()
	windows := previewTestWindows()
	var dec Decorator = NoDecoration{}
	if tableVisible {
		dec = liveFlagDecorator{live: map[string]LiveBadge{"proj-a": {Total: 1}}}
	}
	fetch := func(string) (FetchResult, error) {
		return FetchResult{Sessions: sessions, Decorator: dec, Windows: windows}, nil
	}
	m := New(fetch, dec, nil, nil, showIcons, false, "> ", "Filter sessions...")
	result, _ := m.Update(sessionsLoadedMsg{sessions: sessions, decorator: dec, windows: windows})
	m2 := result.(Model)
	m2.width = 120
	m2.height = 30
	require.Equal(t, tableVisible, m2.showSessionStateTable(), "夹具没有按预期控制住 showSessionStateTable()")
	return m2
}

// stripAndFind 返回 target 在「剥掉 ANSI 转义序列后的 s」里首次出现的可见列位（0-based）；
// 找不到返回 -1。用它来做「结构对齐」断言，而不是逐字节比较带 ANSI 的字符串。
func stripAndFind(s, target string) int {
	stripped := ansi.Strip(s)
	idx := strings.Index(stripped, target)
	if idx < 0 {
		return -1
	}
	return ansi.StringWidth(stripped[:idx])
}

// ---------- Section A: 留白常量三处同步 ----------

func TestBadgeAlignment_ThreeRenderersShareSameStartColumn(t *testing.T) {
	// tc-a26：列头、数据行、表格横线三处渲染实际使用的左内缩数值必须完全相同。
	//
	// 落点选取说明（订正版）：不能用「●」当数据行徽章块起点的落点——ATTN 圆点是在
	// Width(colNumWidth=4).Align(Center) 的 4 宽 cell 里居中渲染的既有代码（本 step
	// 未改），1 个字符居中进 4 宽 cell 会在左侧垫 1 空格，所以「●」天然比块起点靠右 1 列，
	// 这是居中对齐 cell 的正常形态，不是留白算错。改用会话名反推块起点：
	// 块起点 = 会话名起始列位 - colsTotalWidth - badgeRightGap - agentColWidth，两者都是无歧义的块边界。
	m := modelWithStateTable(t, true, true)

	header := renderColumnHeaders(true)
	tableTop := renderTableTop(true, m.contentWidth())
	dec := Decoration{Attention: AttentionBadge{Triggered: true}}
	row := m.renderRow(filteredItem{item: sessionItem{name: "sessionname", src: "tmux", decoration: dec}}, false)

	headerCol := stripAndFind(header, "ATTN")
	tableTopCol := stripAndFind(tableTop, "─")
	rowBlockStart := stripAndFind(row, "sessionname") - colsTotalWidth - badgeRightGap - agentColWidth

	require.NotEqual(t, -1, headerCol)
	require.NotEqual(t, -1, tableTopCol)
	require.NotEqual(t, -1, stripAndFind(row, "sessionname"))
	assert.Equal(t, headerCol, rowBlockStart, "列头 ATTN 首字符列位应与数据行徽章块起始列位一致")
	assert.Equal(t, headerCol, tableTopCol, "表格横线起点列位应与徽章区左内缩起点一致")
}

func TestBadgeAlignment_LeftPadAndRightGapMeetBaselineAndAreConsistent(t *testing.T) {
	// tc-a27/a28：左内缩 >= 2、右间隔 >= 3（原为 1），且列头与数据行取值一致。
	// 同上订正：块起点用会话名反推，不用居中的「●」。
	m := modelWithStateTable(t, false, true) // showIcons=false，去掉图标列这个变量，只看留白本身
	header := renderColumnHeaders(false)
	dec := Decoration{Attention: AttentionBadge{Triggered: true}}
	row := m.renderRow(filteredItem{item: sessionItem{name: "sessionname", src: "tmux", decoration: dec}}, false)

	nameCol := stripAndFind(row, "sessionname")
	require.NotEqual(t, -1, nameCol)
	rowBlockStart := nameCol - colsTotalWidth - badgeRightGap - agentColWidth

	// showIcons=false 时 cursorPrefix(2) 之后立刻是徽章区，左内缩 = 块起点 - 2。
	blockStartFromHeader := stripAndFind(header, "ATTN")
	leftPadFromHeader := blockStartFromHeader - 2
	leftPadFromRow := rowBlockStart - 2
	assert.GreaterOrEqual(t, leftPadFromHeader, 2, "左内缩应 >= 2 空格宽")
	assert.Equal(t, leftPadFromHeader, leftPadFromRow, "列头与数据行的左内缩应一致")

	// 右间隔 = 会话名起始列位 - (块起点 + colsTotalWidth)。
	// 块起点必须取**列头实测**的那个，不能用 rowBlockStart——后者是拿 badgeRightGap
	// 反推出来的，代入化简后 rightGap 会恒等于 badgeRightGap，成为一条永远为真的
	// 重言式，生产代码把 badgeRightGap 换成字面量 1 也测不出来。
	rightGap := nameCol - (blockStartFromHeader + colsTotalWidth + agentColWidth)
	assert.GreaterOrEqual(t, rightGap, 3, "右间隔应 >= 3 空格宽（原为 1）")
}

func TestRenderTableTop_LineLengthFormula(t *testing.T) {
	// tc-a29：横线长度 = contentWidth - leftPad，横线左端与徽章区左内缩起点一致。
	got := renderTableTop(true, 60)
	leftPad := 2 + 2 + badgeLeftPad // 2(cursor占位) + 2(showIcons) + badgeLeftPad
	wantLen := 60 - leftPad

	stripped := ansi.Strip(got)
	require.GreaterOrEqual(t, len(stripped), leftPad)
	assert.Equal(t, wantLen, ansi.StringWidth(stripped[leftPad:]), "横线长度应等于 contentWidth-leftPad")
	assert.Equal(t, leftPad, stripAndFind(got, "─"), "横线起点应与徽章区左内缩起点一致")
}

func TestRenderTableTop_LineLengthFloorsAtColsTotalWidth(t *testing.T) {
	// tc-a29（边界）：contentWidth 很小时，横线长度不应短于 colsTotalWidth(19)。
	got := renderTableTop(false, 10)
	leftPad := 2 + badgeLeftPad
	stripped := ansi.Strip(got)
	lineLen := ansi.StringWidth(stripped) - leftPad
	assert.GreaterOrEqual(t, lineLen, colsTotalWidth)
}

func TestRenderRow_StateTableHidden_NoBadgePadding(t *testing.T) {
	// tc-a30：状态表整体隐藏时，留白完全不生效，会话名紧随来源图标，与改动前一致。
	//
	// 订正：cursorPrefix(2)+srcIcon(2)=4 是"可见列位"，不能用 stripped[4:] 按字节切——
	// 来源图标是多字节 rune（UTF-8 下占 3 字节），字节下标切片会切在图标中间。
	// 改用 stripAndFind 按可见列位定位。
	m := modelWithStateTable(t, true, false)
	row := m.renderRow(filteredItem{item: sessionItem{name: "plain-session", src: "tmux"}}, false)

	assert.Equal(t, 4, stripAndFind(row, "plain-session"),
		"留白不生效时会话名应紧随来源图标（光标 2 + 图标 2），实际 %q", ansi.Strip(row))
}

func TestColumnWidthConstants_UnchangedByBadgePadding(t *testing.T) {
	// tc-a31（回归）：加入 badgeLeftPad/badgeRightGap 不应改变四列自身的宽度常量。
	assert.Equal(t, 5, colCellWidth)
	assert.Equal(t, 4, colLastCellWidth)
	assert.Equal(t, 4, colNumWidth)
	assert.Equal(t, 19, colsTotalWidth)
}

// ---------- Section B: window 行渲染 ----------

func TestRenderWindowRow_ContentFormatAndActiveMarker(t *testing.T) {
	// 行为契约：行内容 "└ " + index + ": " + name；Active 为真时名字后追加 " *"。
	m := newPreviewModel(t, nil)

	inactive := visibleRow{kind: rowWindow, window: WindowItem{Index: 1, Name: "claude", Active: false}}
	active := visibleRow{kind: rowWindow, window: WindowItem{Index: 2, Name: "server", Active: true}}

	outInactive := ansi.Strip(m.renderWindowRow(inactive, false))
	outActive := ansi.Strip(m.renderWindowRow(active, false))

	assert.Contains(t, outInactive, "└ 1: claude")
	assert.NotContains(t, outInactive, "*", "非活动 window 不应带 * 标记")
	assert.Contains(t, outActive, "└ 2: server *")
}

func TestRenderWindowRow_CursorPrefixMatchesSessionRowConvention(t *testing.T) {
	// 光标列与 session 行一致，占 2 列（"> " 或 "  "）。
	m := newPreviewModel(t, nil)
	row := visibleRow{kind: rowWindow, window: WindowItem{Index: 1, Name: "a"}}

	on := ansi.Strip(m.renderWindowRow(row, true))
	off := ansi.Strip(m.renderWindowRow(row, false))

	assert.True(t, strings.HasPrefix(on, "> "), "光标行前缀应是 '> '，实际 %q", on)
	assert.True(t, strings.HasPrefix(off, "  "), "非光标行前缀应是两个空格，实际 %q", off)
}

func TestRenderWindowRow_IndentMatchesNameColStartPlusStep(t *testing.T) {
	// 缩进 = nameColStart + windowIndentStep，
	// nameColStart = (showIcons?2:0) + (showSessionStateTable() ? badgeLeftPad+colsTotalWidth+badgeRightGap : 0)。
	// 2 个布尔维度 × 2 取值 = 4 组合，直接穷举。
	//
	// 订正：stripAndFind 量的是整行的绝对可见列位（含行首的光标列 "  "/"> "），
	// 而 nameColStart 按 plan §8 定义是不含光标列的相对列数，两者之间必须加上
	// 光标列宽度 2 才能对齐——plan §8「列位账本」也是这么算的：
	// 光标列2 + 图标列2 + 徽章区左内缩2 + 四列19 + 右间隔3 = 会话名列起点(列28)，
	// window 行左端 = 28+2 = 列30，与 icons=true,table=true 这组的期望值完全吻合。
	cases := []struct {
		name         string
		showIcons    bool
		tableVisible bool
	}{
		{"icons=false,table=false", false, false},
		{"icons=true,table=false", true, false},
		{"icons=false,table=true", false, true},
		{"icons=true,table=true", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := modelWithStateTable(t, tc.showIcons, tc.tableVisible)
			row := visibleRow{kind: rowWindow, window: WindowItem{Index: 7, Name: "srv"}}
			out := m.renderWindowRow(row, false)

			nameColStart := 0
			if tc.showIcons {
				nameColStart += 2
			}
			if tc.tableVisible {
				nameColStart += badgeLeftPad + colsTotalWidth + badgeRightGap + agentColWidth
			}
			wantIndent := 2 + nameColStart + windowIndentStep // 2 = 光标列

			gotIndent := stripAndFind(out, "└")
			assert.Equal(t, wantIndent, gotIndent,
				"缩进公式 光标列(2)+nameColStart(%d)+windowIndentStep 不符", nameColStart)
		})
	}
}

func TestRenderWindowRow_NoBadgeCharactersOrPlaceholderBlock(t *testing.T) {
	// NEVER 渲染 ATTN/IDLE/RUN/WAIT 任何字符，NEVER 用空白 cell 占位对齐——
	// 布局锚点：window 行不含四个字样；正文（缩进之后）不含用于占位的连续 19 空格块。
	//
	// 订正：按 plan §8 列位账本，showIcons=true+状态表可见时 window 行左端本就该在
	// 列 30（缩进=光标列2+nameColStart(26)+windowIndentStep(2)=30 个前导空格），
	// 这本身就必然包含 19 个连续空格——缩进量是账本明确要求的，不是占位违规。
	// 契约原文禁的是"渲染一个 19 宽的徽章 cell 假装那里有内容"，不是禁止缩进本身。
	// 因此只检查缩进之后的正文：紧跟 └，正文里不再额外出现 19 连续空格的占位块。
	m := modelWithStateTable(t, true, true)
	row := visibleRow{kind: rowWindow, window: WindowItem{Index: 1, Name: "claude"}}
	out := ansi.Strip(m.renderWindowRow(row, false))

	for _, forbidden := range []string{"ATTN", "IDLE", "RUN", "WAIT"} {
		assert.NotContains(t, out, forbidden)
	}
	body := strings.TrimLeft(out, " ")
	assert.True(t, strings.HasPrefix(body, "└"), "缩进之后应立刻是 └，中间不得有占位块，实际 %q", out)
	assert.NotContains(t, body, strings.Repeat(" ", colsTotalWidth),
		"正文里不应含用于占位对齐的连续 19 空格块")
}

func TestRenderWindowRow_SearchHighlightMatchesSessionRowStyle(t *testing.T) {
	// 搜索高亮：matchedIndexes 非空时用与 session 名相同的 matchStyle，走既有 highlightMatches。
	// 直接复用 highlightMatches + colorMatch 算出"session 名会被高亮成什么样"，
	// 断言 window 行里出现完全相同的一段，而不是自己猜一段 ANSI 转义码去比较。
	m := newPreviewModel(t, nil)
	name := "claude"
	matched := []int{0, 2}

	matchStyle := lipgloss.NewStyle().Foreground(colorMatch).Bold(true)
	wantHighlighted := highlightMatches(name, matched, matchStyle, lipgloss.NewStyle())

	row := visibleRow{kind: rowWindow, window: WindowItem{Index: 1, Name: name}, matchedIndexes: matched}
	out := m.renderWindowRow(row, false)

	assert.Contains(t, out, wantHighlighted, "window 行的搜索高亮应与 session 名同款 matchStyle 逐字符一致")
}

func TestRenderWindowRow_NeverRendersAttentionTail(t *testing.T) {
	// window 行 NEVER 渲染 ATTN 行尾注（done Nm ago）。
	m := newPreviewModel(t, nil)
	row := visibleRow{kind: rowWindow, window: WindowItem{Index: 1, Name: "claude"}}
	out := ansi.Strip(m.renderWindowRow(row, false))

	assert.NotContains(t, out, "done")
	assert.NotContains(t, out, "ago")
}

// ---------- Section C: 预览分栏布局 ----------

func TestShowPreview_BelowThreshold_False(t *testing.T) {
	m := newPreviewModel(t, nil)
	m.width = 101
	assert.False(t, m.showPreview())
}

func TestShowPreview_AtThreshold_True(t *testing.T) {
	// 边界：width == previewMinTotal(102) 时应显示
	m := newPreviewModel(t, nil)
	m.width = 102
	assert.True(t, m.showPreview())
}

func TestShowPreview_TogglesWithWindowSizeAndNeverCached(t *testing.T) {
	// 布局锚点：101→102 预览出现；102→101 预览消失；再拉宽自动恢复。
	// NEVER 缓存判定结果——用同一个 Model 实例反复走 tea.WindowSizeMsg 验证每次都现算。
	m := newPreviewModel(t, &fakeCapturer{content: "p"})
	m.cursor = idxProjA

	r1, _ := m.Update(tea.WindowSizeMsg{Width: 101, Height: 30})
	m1 := r1.(Model)
	assert.False(t, m1.showPreview())

	r2, _ := m1.Update(tea.WindowSizeMsg{Width: 102, Height: 30})
	m2 := r2.(Model)
	assert.True(t, m2.showPreview())

	r3, _ := m2.Update(tea.WindowSizeMsg{Width: 101, Height: 30})
	m3 := r3.(Model)
	assert.False(t, m3.showPreview(), "拉窄后应立即消失")

	r4, _ := m3.Update(tea.WindowSizeMsg{Width: 102, Height: 30})
	m4 := r4.(Model)
	assert.True(t, m4.showPreview(), "再拉宽应自动恢复")
}

func TestPreviewWidth_Formula(t *testing.T) {
	// tc-a75（比例分宽版）：previewWidth() = width - contentWidth() - previewGap；
	// 分栏时 contentWidth() = max(listBaseWidth, width*listRatio%)，
	// 故列表 40% / 预览约 60%，且 previewWidth()>=40 恒成立。
	cases := []struct {
		width    int
		wantList int
		want     int
	}{
		{102, 60, 40},  // 边界：40% < 60 → 列表取下限 60，预览取下限 40
		{120, 60, 58},  // tc-a75 原题：40%=48 < 60 → 列表 60 + 间距 2 + 预览 58
		{150, 60, 88},  // 恰好 40% == 60：比例与下限的换挡点
		{160, 64, 94},  // 40% 起作用：列表 64，预览 94
		{200, 80, 118}, // 列表 80 / 预览 118 ≈ 4:6
	}
	for _, tc := range cases {
		m := newPreviewModel(t, nil)
		m.width = tc.width
		assert.Equal(t, tc.wantList, m.contentWidth(), "contentWidth width=%d", tc.width)
		assert.Equal(t, tc.want, m.previewWidth(), "previewWidth width=%d", tc.width)
		assert.GreaterOrEqual(t, m.previewWidth(), previewMinWidth)
	}
}

func TestPreviewHeight_UsesTerminalHeightNotVisibleCountCap(t *testing.T) {
	// 预览块与整个列表块并排，从视图顶部可用到底：高度按终端给（留 1 行余量），
	// 不绑 visibleCount()——后者要扣列表上方的 chrome 行数，预览没有这份开销。
	m := newPreviewModel(t, nil)
	m.height = 40
	assert.Equal(t, 39, m.previewHeight())
	assert.Greater(t, m.previewHeight(), m.visibleCount(), "预览高度不应被列表的 chrome 预算拖住")

	m.height = 0 // WindowSizeMsg 未到
	assert.Equal(t, 3, m.previewHeight(), "高度未知时用下限兜底")
}

func TestVisibleCount_ScalesWithTerminalHeightWithoutCap(t *testing.T) {
	// 列表行数随终端高度伸缩，不再有 15 行 cap（70% 高 popup 时代的遗产）。
	// newPreviewModel 的夹具无 Claude 实例 → 状态表隐藏，chrome = 12-2 = 10
	m := newPreviewModel(t, nil)
	m.height = 40
	assert.Equal(t, 30, m.visibleCount(), "高 popup 下列表应吃满可用高度（40 - chrome 10）")

	m.height = 22
	assert.Equal(t, 12, m.visibleCount(), "矮终端下仍按 height-chrome 算")
}

func TestVisibleCount_UnaffectedByPreviewVisibility(t *testing.T) {
	// tc-a76：预览分栏不占用列表可见行数——同一 height 下，预览可见/隐藏两态的
	// 列表可见条数必须完全一致。
	m := newPreviewModel(t, nil)
	m.height = 30

	m.width = 60 // 无预览
	countNoPreview := m.visibleCount()
	require.False(t, m.showPreview())

	m.width = 200 // 有预览
	countWithPreview := m.visibleCount()
	require.True(t, m.showPreview())

	assert.Equal(t, countNoPreview, countWithPreview)
}

func TestView_Width101_MatchesNoPreviewOutput(t *testing.T) {
	// 覆盖锚点「宽度==101 时输出为单列且与无预览版本一致」：
	// 配置了 Capturer 但宽度不足阈值时的 View() 输出，应与完全没有配置 Capturer 时
	// 逐字节相同——证明预览列在这个宽度下根本没有被拼接进去。
	withCap := newPreviewModel(t, &fakeCapturer{content: "x"})
	withCap.width = 101
	withCap.cursor = idxProjA

	withoutCap := newPreviewModel(t, nil)
	withoutCap.width = 101
	withoutCap.cursor = idxProjA

	assert.Equal(t, withoutCap.View().Content, withCap.View().Content)
}

// ---------- Section D: 取快照六步 + PaneCapturer 覆盖锚点 ----------

func TestRetargetPreview_HappyPathThroughAllSixSteps(t *testing.T) {
	// tc-a32 + tc-a34 + tc-a36 + 覆盖锚点①：完整走一遍六步。
	fakeCap := &fakeCapturer{content: "hello preview"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	cmd := m.retargetPreview(false)
	require.NotNil(t, cmd)

	// step1：当帧就该清空旧画面并进入读取中。
	loadingFrame := ansi.Strip(m.renderPreview(40, 5))
	assert.Contains(t, loadingFrame, previewLoadingText)

	// step2/3：真的通过 tea.Tick 拿到 previewTickMsg（真实阻塞约 150ms）。
	msg := cmd()
	tick, ok := msg.(previewTickMsg)
	require.True(t, ok, "cmd() 应产出 previewTickMsg，实际 %T", msg)
	wantTarget, hasTarget := m.previewTarget()
	require.True(t, hasTarget)
	assert.Equal(t, wantTarget, tick.target)

	// step4/5：序号匹配 → 应该发起抓屏 cmd。
	result, captureCmd := m.Update(tick)
	m = result.(Model)
	require.NotNil(t, captureCmd, "序号匹配的 tick 应该发起抓屏 cmd")

	resMsg := captureCmd()
	res, ok := resMsg.(previewResultMsg)
	require.True(t, ok, "抓屏 cmd 应产出 previewResultMsg，实际 %T", resMsg)
	assert.Equal(t, tick.seq, res.seq)
	assert.Equal(t, tick.target, res.target)
	assert.Equal(t, "hello preview", res.content)
	require.Len(t, fakeCap.calls, 1)
	assert.Equal(t, wantTarget, fakeCap.calls[0])

	// step6：序号与目标都对上 → 更新内容，读取中占位消失。
	result2, _ := m.Update(res)
	m2 := result2.(Model)
	rendered := ansi.Strip(m2.renderPreview(40, 5))
	assert.Contains(t, rendered, "hello preview")
	assert.NotContains(t, rendered, previewLoadingText)
}

func TestRetargetPreview_StaleTickDiscarded(t *testing.T) {
	// tc-a33 + 覆盖锚点⑥：定时消息 seq 过期 → 不抓屏、Capture 零调用。
	fakeCap := &fakeCapturer{content: "x"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	cmdA := m.retargetPreview(false)
	require.NotNil(t, cmdA)
	tickA := cmdA().(previewTickMsg) // 真实等 150ms，拿到 A 这一轮真实的 seq

	// 光标移动到 B，产生新一轮 retarget，seq 必然递增，tickA 现在已经过期。
	m.cursor = idxProjB
	_ = m.retargetPreview(false)

	result, nextCmd := m.Update(tickA)
	m2 := result.(Model)
	_ = m2

	assert.Nil(t, nextCmd, "序号过期的 tick 不应发起抓屏 cmd")
	assert.Empty(t, fakeCap.calls, "序号过期不该调用 Capture")
}

func TestRetargetPreview_StaleResultSameTargetDiscarded(t *testing.T) {
	// tc-a35：抓屏结果 seq 过期但 target 与当前一致（光标离开又碰巧回到同一目标）→ 丢弃。
	fakeCap := &fakeCapturer{content: "z"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	cmdA := m.retargetPreview(false)
	tickA := cmdA().(previewTickMsg) // 真实等 150ms，学到 A 这一轮真实 seq
	targetA := tickA.target

	m.cursor = idxProjB
	_ = m.retargetPreview(false) // 离开 A，seq 递增
	m.cursor = idxProjA
	_ = m.retargetPreview(false) // 又回到 A，seq 再次递增；此时内部真实 seq 已比 tickA.seq 大两轮

	staleResult := previewResultMsg{seq: tickA.seq, target: targetA, content: "should-not-appear"}
	result, _ := m.Update(staleResult)
	m2 := result.(Model)

	rendered := ansi.Strip(m2.renderPreview(40, 5))
	assert.NotContains(t, rendered, "should-not-appear", "序号过期的结果即使目标串相同也不该采信")
}

func TestRetargetPreview_ResultWithMismatchedTargetDiscardedEvenIfSeqMatches(t *testing.T) {
	// tc-a73：结果 seq 与当前一致，但 target 对不上 → 仍需丢弃（防止"抓 A 的结果画在 B 行上"）。
	fakeCap := &fakeCapturer{content: "z"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	cmdA := m.retargetPreview(false)
	tickA := cmdA().(previewTickMsg) // 真实等 150ms；此刻内部 seq 恰好等于 tickA.seq

	wrongTargetResult := previewResultMsg{seq: tickA.seq, target: "totally-different-target", content: "should-not-appear"}
	result, _ := m.Update(wrongTargetResult)
	m2 := result.(Model)

	rendered := ansi.Strip(m2.renderPreview(40, 5))
	assert.NotContains(t, rendered, "should-not-appear", "序号对上但目标串不一致仍应丢弃")
}

func TestRetargetPreview_DuplicateTargetSkipsRefetch(t *testing.T) {
	// tc-a37 + 覆盖锚点⑦：目标未变、不在 loading、已有快照 → 整个流程跳过，不重取。
	fakeCap := &fakeCapturer{content: "cached"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	result, captureCmd := m.Update(tick)
	m = result.(Model)
	res := captureCmd().(previewResultMsg)
	result2, _ := m.Update(res)
	m = result2.(Model)
	require.Len(t, fakeCap.calls, 1)

	again := m.retargetPreview(false)
	assert.Nil(t, again, "目标未变且已有快照时应整体跳过")
	assert.Len(t, fakeCap.calls, 1, "重复目标不应重新调用 Capture")
}

func TestRetargetPreview_CtrlRForcesRefetchEvenWithExistingContent(t *testing.T) {
	// tc-a38 + 覆盖锚点⑧：Ctrl-r 无条件重新走完六步，Capture 再被调用一次。
	fakeCap := &fakeCapturer{content: "v1"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	result, captureCmd := m.Update(tick)
	m = result.(Model)
	res := captureCmd().(previewResultMsg)
	result2, _ := m.Update(res)
	m = result2.(Model)
	require.Len(t, fakeCap.calls, 1)

	forceCmd := m.retargetPreview(true)
	require.NotNil(t, forceCmd, "Ctrl-r 应无条件重新走完六步，不受'重复目标不重取'规则影响")
	tick2 := forceCmd().(previewTickMsg)
	assert.NotEqual(t, tick.seq, tick2.seq, "seq 应自增，让旧结果自然作废")

	result3, captureCmd2 := m.Update(tick2)
	m = result3.(Model)
	require.NotNil(t, captureCmd2)
	res2 := captureCmd2().(previewResultMsg)
	_, _ = m.Update(res2)

	assert.Len(t, fakeCap.calls, 2, "Ctrl-r 应让 Capture 再被调用一次")
}

func TestRetargetPreview_RapidCursorMovementOnlyFinalTargetFetches(t *testing.T) {
	// tc-a39：连续快速经过多个目标，只有最后停留者真正抓屏。
	fakeCap := &fakeCapturer{content: "final"}
	m := newPreviewModel(t, fakeCap)

	m.cursor = idxProjA
	cmdA := m.retargetPreview(false) // A 的 tick 从未被 fire，模拟"停留不足 150ms 就离开"
	m.cursor = idxProjB
	_ = m.retargetPreview(false) // B 同样被跳过
	m.cursor = idxProjA
	cmdC := m.retargetPreview(false) // 最终又停在 proj-a（复用作为"停留超过 150ms 的 C"）

	require.NotNil(t, cmdC)
	tickC := cmdC().(previewTickMsg) // 只有这一次真正等了 150ms 触发
	result, captureCmd := m.Update(tickC)
	m = result.(Model)
	require.NotNil(t, captureCmd)
	res := captureCmd().(previewResultMsg)
	result2, _ := m.Update(res)
	m = result2.(Model)

	require.Len(t, fakeCap.calls, 1, "只有最终停留的目标应该真正发起抓屏")

	// A 的 tick 即使之后才被喂进来，也该因为 seq 过期被丢弃。
	tickA := cmdA().(previewTickMsg)
	result3, cmd3 := m.Update(tickA)
	_ = result3.(Model)
	assert.Nil(t, cmd3, "过期的 A 不应再发起抓屏")
	assert.Len(t, fakeCap.calls, 1, "过期的 A 不应让 Capture 总调用次数增加")
}

func TestPreviewResult_CaptureError_ShowsFailureAndDoesNotQuit(t *testing.T) {
	// 覆盖锚点②：Capture 返回 error → err 落库并渲染失败说明，picker 不退出、列表仍可操作。
	fakeCap := &fakeCapturer{err: errors.New("tmux: pane not found")}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	result, captureCmd := m.Update(tick)
	m = result.(Model)
	res := captureCmd().(previewResultMsg)
	assert.NotEmpty(t, res.err)

	result2, cmd2 := m.Update(res)
	m2 := result2.(Model)
	assert.Nil(t, cmd2, "抓屏失败不应触发退出")
	assert.False(t, m2.Quit())

	rendered := ansi.Strip(m2.renderPreview(40, 5))
	assert.Contains(t, rendered, previewFailedPrefix)
	assert.Contains(t, rendered, "tmux: pane not found")

	// 列表仍可操作：按键不应 panic，光标仍能正常处理移动。
	require.NotPanics(t, func() {
		_, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	})
}

func TestRetargetPreview_NonTmuxCursorRow_NoCommandNoCapture(t *testing.T) {
	// 覆盖锚点⑤：光标停在非 tmux 项 → Capture 零调用，不发起任何取快照流程。
	fakeCap := &fakeCapturer{content: "should-not-be-used"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxCfgapp

	cmd := m.retargetPreview(false)

	assert.Nil(t, cmd, "无目标时不应发起任何取快照流程")
	assert.Empty(t, fakeCap.calls)
	assert.Contains(t, ansi.Strip(m.renderPreview(40, 5)), previewNoTargetText)
}

func TestView_NeverCallsCaptureSynchronously(t *testing.T) {
	// NEVER 在渲染路径上同步抓屏：单纯调用 View() 不应触发 Capture。
	fakeCap := &fakeCapturer{content: "x"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	_ = m.View()

	assert.Empty(t, fakeCap.calls, "View() 渲染路径上不应同步调用 Capture")
}

// ---------- Section E: 目标解析 ----------

func TestPreviewTarget_SessionRowWithActiveWindow_ResolvesToActiveIndex(t *testing.T) {
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA // proj-a 的 Index2 是活动 window

	target, ok := m.previewTarget()
	require.True(t, ok)
	assert.Equal(t, "proj-a:2", target)
}

func TestPreviewTarget_SessionRowWithoutActiveWindow_FallsBackToSmallestIndex(t *testing.T) {
	// tc-a74：没有任何 window 被标记为活动 window → 退回按 Index 升序排列后的第一项。
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjB // proj-b 的 window 是 Index 5、2，均非活动

	target, ok := m.previewTarget()
	require.True(t, ok)
	assert.Equal(t, "proj-b:2", target, "无活动 window 时应退回 Index 最小的那个（2，不是声明顺序里的 5）")
}

func TestPreviewTarget_SessionRowWithEmptyWindowTable_NoTarget(t *testing.T) {
	// session 行但 window 表为空 → 无目标（按非 tmux 项处理）
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjC // tmux 来源但 0 个 window

	_, ok := m.previewTarget()
	assert.False(t, ok)
}

func TestPreviewTarget_NonTmuxSessionRow_NoTarget(t *testing.T) {
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxCfgapp // config 来源

	_, ok := m.previewTarget()
	assert.False(t, ok)
}

func TestPreviewTarget_WindowRow_ResolvesToSessionColonIndex(t *testing.T) {
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	m.expandCurrent()
	m.cursor = 1 // proj-a 展开后第一个 window 行（Index=1）

	target, ok := m.previewTarget()
	require.True(t, ok)
	assert.Equal(t, "proj-a:1", target)
}

func TestPreviewTarget_EmptyFilteredList_NoTarget(t *testing.T) {
	// 无匹配结果（空列表）→ 无目标
	m := newPreviewModel(t, &fakeCapturer{})
	m.filterInput.SetValue("zzz-no-match")
	m.applyFilter()

	_, ok := m.previewTarget()
	assert.False(t, ok)
}

func TestPreviewTarget_NilCapturer_NoTarget(t *testing.T) {
	// Capturer == nil → 无目标（保证未注入时既有测试行为不变）
	m := newPreviewModel(t, nil)
	m.cursor = idxProjA // 光标本身停在一个本可正常解析出目标的行上

	_, ok := m.previewTarget()
	assert.False(t, ok, "Capturer 未注入时不应解析出任何目标")
}

// ---------- Section F: 预览内容渲染 ----------

// firePreviewToContent 走完一遍取快照六步，把 content 落进 m 的预览状态里，
// 返回渲染出来的整块预览文本，供 renderPreview 相关测试直接复用（避免每个测试
// 都重复这一长串真实的 tea.Tick 等待流程）。
func firePreviewToContent(t *testing.T, m *Model, content string) {
	t.Helper()
	cmd := m.retargetPreview(false)
	require.NotNil(t, cmd)
	tick := cmd().(previewTickMsg)
	result, captureCmd := m.Update(tick)
	*m = result.(Model)
	require.NotNil(t, captureCmd)
	res := captureCmd().(previewResultMsg)
	res.content = content // 复用同一条真实 seq/target，只替换内容，方便按测试场景定制文本
	result2, _ := m.Update(res)
	*m = result2.(Model)
}

func TestRenderPreview_LoadingState(t *testing.T) {
	m := newPreviewModel(t, &fakeCapturer{content: "x"})
	m.cursor = idxProjA
	m.retargetPreview(false) // 只需要进入 loading，不必等 tick 真的触发

	rendered := ansi.Strip(m.renderPreview(40, 5))
	assert.Contains(t, rendered, previewLoadingText)
}

func TestRenderPreview_ErrorState(t *testing.T) {
	fakeCap := &fakeCapturer{err: errors.New("boom")}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	result, captureCmd := m.Update(tick)
	m = result.(Model)
	res := captureCmd().(previewResultMsg)
	result2, _ := m.Update(res)
	m = result2.(Model)

	rendered := ansi.Strip(m.renderPreview(40, 5))
	assert.Contains(t, rendered, previewFailedPrefix+"boom")
}

func TestRenderPreview_NoTargetState(t *testing.T) {
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxCfgapp // 非 tmux，无目标
	m.retargetPreview(false)

	rendered := ansi.Strip(m.renderPreview(40, 5))
	assert.Contains(t, rendered, previewNoTargetText)
}

func TestRenderPreview_TruncatesLongLineWithoutBreakingAnsi(t *testing.T) {
	// tc-a68：预览区宽度 40 列，行内可见字符宽度 55 列且带 ANSI 颜色，
	// 截断后可见宽度不超过 40 列，且转义序列完整（Strip 后文本是正确的截断前缀）。
	longVisible := strings.Repeat("a", 55)
	coloredLine := "\x1b[31m" + longVisible + "\x1b[0m" // 红色，可见宽度 55
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, coloredLine)

	rendered := m.renderPreview(40, 1)
	lines := strings.Split(rendered, "\n")
	require.NotEmpty(t, lines)
	line := lines[0]

	assert.LessOrEqual(t, ansi.StringWidth(line), 40, "截断后可见宽度不应超过 previewWidth")
	stripped := ansi.Strip(line)
	assert.True(t, strings.HasPrefix(longVisible, stripped) || strings.HasPrefix(stripped, strings.Repeat("a", 40)),
		"截断只应裁剪可见字符本身，不应产生乱码，实际 %q", stripped)
	// 用 rune/[]byte 切片截断会把 "\x1b[31m...\x1b[0m" 从中间切断，导致 Strip 都无法正确解析出
	// 纯 'a' 序列（会残留半截转义码的字节）；ansi 包感知截断则能被 Strip 干净地还原。
	assert.Regexp(t, `^a*$`, stripped, "转义序列必须保持完整，Strip 后应该是纯 'a' 序列，不应残留转义码碎片")
}

func TestRenderPreview_KeepsTrailingLinesWhenContentExceedsHeight(t *testing.T) {
	// tc-a69：预览区高度 10 行，内容共 30 行 → 渲染第 21～30 行（最末 10 行）。
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i+1)
	}
	content := strings.Join(lines, "\n")

	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, content)

	rendered := ansi.Strip(m.renderPreview(40, 10))
	for i := 1; i <= 20; i++ {
		assert.NotContains(t, rendered, "line"+strconv.Itoa(i)+"\n", "第 %d 行应被舍弃", i)
	}
	for i := 21; i <= 30; i++ {
		assert.Contains(t, rendered, "line"+strconv.Itoa(i), "应保留第 %d 行", i)
	}
}

func TestRenderPreview_StripsTrailingBlankLinesBeforeKeepingTail(t *testing.T) {
	// 抓屏回归：capture-pane 返回整个 pane 高度，内容贴顶的 pane 尾部是成片空行。
	// 若不先裁空尾行，「保留末尾 height 行」留下的全是空白，真内容被裁走。
	// 构造：5 行真内容 + 25 行空行（含纯空格和带 ANSI 的"视觉空行"），高度 10。
	lines := make([]string, 0, 30)
	for i := 1; i <= 5; i++ {
		lines = append(lines, "content"+strconv.Itoa(i))
	}
	for range 24 {
		lines = append(lines, "")
	}
	lines = append(lines, "\x1b[0m   ") // 带转义与空格的视觉空行也要算空
	content := strings.Join(lines, "\n")

	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, content)

	rendered := ansi.Strip(m.renderPreview(40, 10))
	for i := 1; i <= 5; i++ {
		assert.Contains(t, rendered, "content"+strconv.Itoa(i), "真内容第 %d 行不应被空尾行挤掉", i)
	}
}

func TestSanitizePreviewLine_KeepsOnlyTextAndSgr(t *testing.T) {
	// 白名单清洗：非 SGR 转义与零宽字符会让差分渲染器与终端的宽度/光标模型
	// 产生分歧，一格之差整屏错位（长列表导航乱屏的成因之一）。
	cases := []struct {
		name, in, want string
	}{
		{"SGR 颜色保留", "\x1b[31mred\x1b[0m", "\x1b[31mred\x1b[0m"},
		{"OSC8 超链接剥掉", "\x1b]8;;https://x.dev\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"OSC BEL 终止形态", "\x1b]0;title\x07text", "text"},
		{"光标移动剥掉", "a\x1b[5Zb\x1b[10Cc\x1b[2Ad", "abcd"},
		{"字符集切换剥掉", "\x1b(0qqq\x1b(B", "qqq"},
		{"VS16 变体选择符剥掉", "⚠️!", "⚠!"},
		{"零宽连接符剥掉", "a‍b", "ab"},
		{"tab 换空格", "a\tb", "a b"},
		{"CR 剥掉", "line\r", "line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizePreviewLine(tc.in))
		})
	}
}

func TestRenderPreview_ResetsStyleBeforeEachLine(t *testing.T) {
	// tc-a70：前一行未闭合的转义序列不应污染下一行——每行渲染前前置 "\x1b[0m"。
	content := "\x1b[31mred-unclosed\nsecond-line"
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, content)

	rendered := m.renderPreview(40, 2)
	lines := strings.Split(rendered, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	for i, line := range lines[:2] {
		assert.True(t, strings.HasPrefix(line, "\x1b[0m"), "第 %d 行渲染前应重置样式，实际 %q", i, line)
	}
}

func TestRenderPreview_ExactWidthLineNotTruncated(t *testing.T) {
	// tc-a71：行宽恰好等于预览区宽度时不发生截断，整行内容完整保留。
	exact := strings.Repeat("b", 40)
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, exact)

	rendered := m.renderPreview(40, 1)
	lines := strings.Split(rendered, "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, exact, ansi.Strip(lines[0]), "恰好等宽时不应截断")
}

func TestRenderPreview_PadsShortContentToFullHeight(t *testing.T) {
	// 不足高度时用空行补齐，保证分栏块高度稳定（否则 JoinHorizontal 会让列表区跳动）。
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxProjA
	firePreviewToContent(t, &m, "only one line")

	rendered := m.renderPreview(40, 6)
	lines := strings.Split(rendered, "\n")
	assert.Len(t, lines, 6, "内容不足高度时应补齐空行，保证块高度稳定")
}

// ---------- Section H: 可交互控件 ----------

func TestUpdateCtrlR_TriggersRetarget(t *testing.T) {
	// 覆盖锚点「Ctrl-r：seq 自增 + loading 置真 + 重新发起抓屏」
	fakeCap := &fakeCapturer{content: "v"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	r1, cmd1 := m.Update(tick)
	m = r1.(Model)
	res := cmd1().(previewResultMsg)
	r2, _ := m.Update(res)
	m = r2.(Model)
	require.Len(t, fakeCap.calls, 1)

	result3, cmd3 := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m3 := result3.(Model)
	require.NotNil(t, cmd3, "Ctrl-r 应该触发新一轮取快照")
	rendered := ansi.Strip(m3.renderPreview(40, 5))
	assert.Contains(t, rendered, previewLoadingText, "Ctrl-r 应立即清空旧画面并进入读取中")
}

func TestUpdateCtrlR_NoTarget_NoOp(t *testing.T) {
	// 覆盖锚点「Ctrl-r：无目标时无反应」
	m := newPreviewModel(t, &fakeCapturer{})
	m.cursor = idxCfgapp // 非 tmux，无目标

	result, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	_ = result.(Model)
	assert.Nil(t, cmd, "无目标时 Ctrl-r 应无反应")
}

func TestUpdateArrowDown_ClearsPreviewImmediatelyOnCursorMove(t *testing.T) {
	// 覆盖锚点「↑/↓ 移动光标：预览立刻清空并进入读取中（旧画面绝不残留）」
	fakeCap := &fakeCapturer{content: "old-content"}
	m := newPreviewModel(t, fakeCap)
	m.cursor = idxProjA

	tick := m.retargetPreview(false)().(previewTickMsg)
	r1, cmd1 := m.Update(tick)
	m = r1.(Model)
	res := cmd1().(previewResultMsg)
	r2, _ := m.Update(res)
	m = r2.(Model)
	require.Contains(t, ansi.Strip(m.renderPreview(40, 5)), "old-content")

	result3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m3 := result3.(Model)
	rendered := ansi.Strip(m3.renderPreview(40, 5))
	assert.NotContains(t, rendered, "old-content", "光标移动后旧画面绝不能残留")
	assert.Contains(t, rendered, previewLoadingText)
}
