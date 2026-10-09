package picker

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentBadgesAndWindowAlignment(t *testing.T) {
	for _, tc := range []struct {
		badge AgentBadge
		want  string
	}{
		{AgentBadge{}, "       "}, {AgentBadge{CC: true}, "cc     "}, {AgentBadge{CX: true}, "cx     "}, {AgentBadge{CC: true, CX: true}, "cc cx  "},
	} {
		require.Equal(t, tc.want, ansi.Strip(renderAgents(tc.badge)))
	}
	m := modelWithStateTable(t, false, true)
	badge := AgentBadge{CC: true, CX: true}
	session := ansi.Strip(m.renderRow(filteredItem{item: sessionItem{name: "project", src: "tmux", decoration: Decoration{Agents: badge, Live: LiveBadge{Total: 2, Busy: 1}}}}, false))
	window := ansi.Strip(m.renderWindowRow(visibleRow{kind: rowWindow, window: WindowItem{Name: "shell", Index: 1, Agents: badge}}, false))
	require.Equal(t, stripAndFind(session, "cc cx"), stripAndFind(window, "cc cx"))
	require.Contains(t, window, "└ 1: shell")
	require.Contains(t, ansi.Strip(renderColumnHeaders(false)), "AGENT")
}
