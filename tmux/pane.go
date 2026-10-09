package tmux

import (
	"fmt"
	"strings"

	"github.com/Wingsdh/cc-sesh/v2/convert"
	"github.com/Wingsdh/cc-sesh/v2/model"
)

func listpanesformat() string {
	variables := []string{
		"#{window_index}",
		"#{window_name}",
		"#{pane_index}",
		"#{pane_title}",
		"#{pane_current_command}",
		"#{pane_current_path}",
		"#{pane_id}",
	}
	return strings.Join(variables, separator)
}

func (t *RealTmux) ListTmuxPanes() ([]*model.TmuxPane, error) {
	output, err := t.shell.ListCmd("tmux", "list-panes", "-s", "-F", listpanesformat())
	if err != nil {
		return []*model.TmuxPane{}, nil
	}
	return parseTmuxPanesOutput(output)
}

func parseTmuxPanesOutput(rawList []string) ([]*model.TmuxPane, error) {
	panes := make([]*model.TmuxPane, 0, len(rawList))
	for _, line := range rawList {
		fields := strings.Split(line, separator)
		if len(fields) != 7 {
			continue
		}
		panes = append(panes, &model.TmuxPane{
			WindowIndex: convert.StringToInt(fields[0]),
			WindowName:  fields[1],
			PaneIndex:   convert.StringToInt(fields[2]),
			PaneTitle:   fields[3],
			PaneCommand: fields[4],
			PanePath:    fields[5],
			PaneID:      fields[6],
		})
	}
	return panes, nil
}

// ListWindowPanes 列出目标 window 内全部 pane 的序号，顺序沿用 tmux 输出。
// 预览分栏用它枚举 window 的 pane 逐个抓屏；失败 fail-soft 返回空切片 + nil，
// 调用方退回「只抓活动 pane」即可，不值得为预览断流。
func (t *RealTmux) ListWindowPanes(target string) ([]int, error) {
	output, err := t.shell.ListCmd("tmux", "list-panes", "-t", target, "-F", "#{pane_index}")
	if err != nil {
		return []int{}, nil
	}
	indexes := make([]int, 0, len(output))
	for _, line := range output {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		indexes = append(indexes, convert.StringToInt(line))
	}
	return indexes, nil
}

// ListAllPanes 跨所有 tmux session 列出 pane（cwd / pid / 所属 session）。
// 用于 cc-sesh 的 Claude → session 关联：通过 cwd 匹配。
func (t *RealTmux) ListAllPanes() ([]*model.TmuxPaneAcrossSessions, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{pane_id}",
		"#{pane_current_path}",
		"#{pane_pid}",
		"#{window_index}",
	}, separator)
	output, err := t.shell.ListCmd("tmux", "list-panes", "-a", "-F", format)
	if err != nil {
		return []*model.TmuxPaneAcrossSessions{}, nil
	}
	out := make([]*model.TmuxPaneAcrossSessions, 0, len(output))
	for _, line := range output {
		fields := strings.Split(line, separator)
		if len(fields) != 4 && len(fields) != 5 {
			continue
		}
		windowIndex := 0
		if len(fields) == 5 {
			windowIndex = convert.StringToInt(fields[4])
		}
		out = append(out, &model.TmuxPaneAcrossSessions{
			WindowIndex:     windowIndex,
			SessionName:     fields[0],
			PaneID:          fields[1],
			PaneCurrentPath: fields[2],
			PanePID:         convert.StringToInt(fields[3]),
		})
	}
	return out, nil
}

func (t *RealTmux) SelectPane(windowIndex int, paneIndex int) (string, error) {
	sessionName, err := t.GetCurrentSession()
	if err != nil {
		return "", fmt.Errorf("failed to get current session: %w", err)
	}

	target := fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, paneIndex)
	if _, err := t.shell.Cmd("tmux", "select-window", "-t", fmt.Sprintf("%s:%d", sessionName, windowIndex)); err != nil {
		return "", fmt.Errorf("failed to select window %d: %w", windowIndex, err)
	}
	if _, err := t.shell.Cmd("tmux", "select-pane", "-t", target); err != nil {
		return "", fmt.Errorf("failed to select pane %d in window %d: %w", paneIndex, windowIndex, err)
	}
	return fmt.Sprintf("selected pane %d in window %d", paneIndex, windowIndex), nil
}

func (t *RealTmux) GetCurrentSession() (string, error) {
	return t.shell.Cmd("tmux", "display-message", "-p", "#{session_name}")
}

// ListClients 返回当前所有 tmux client attach 到的 session 名（已去重）。
// 用于 attention：被任何 client attach 的 session 都视作"用户正在看"，
// 不应再写 attention flag，已存在的 flag 也会被清掉。
//
// 用 t.shell.Cmd 而非 ListCmd：前者已经把 tmux "no server running" 特判成
// ("", nil)，正好对应"没启动 tmux 也没有 client"的预期；真正的错误
// （比如 tmux 二进制不存在、socket 权限）才会冒出来给调用方记日志。
func (t *RealTmux) ListClients() ([]string, error) {
	output, err := t.shell.Cmd(t.bin, "list-clients", "-F", "#{client_session}")
	if err != nil {
		return nil, err
	}
	if output == "" {
		return nil, nil
	}
	lines := strings.Split(output, "\n")
	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}
