package seshcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Wingsdh/cc-sesh/v2/claude/live"
	codexlive "github.com/Wingsdh/cc-sesh/v2/codex/live"
	"github.com/Wingsdh/cc-sesh/v2/lister"
	"github.com/Wingsdh/cc-sesh/v2/model"
	"github.com/Wingsdh/cc-sesh/v2/picker"
	"github.com/Wingsdh/cc-sesh/v2/tmux"
)

type changingCodexReader struct {
	statuses []live.Status
	index    int
}

func (r *changingCodexReader) Read(panes []codexlive.Pane) (map[string]live.Status, error) {
	if len(panes) != 1 || panes[0].PID != 100 || panes[0].SessionName != "alpha" {
		return nil, nil
	}
	status := r.statuses[r.index]
	r.index++
	return map[string]live.Status{"alpha": status}, nil
}

func TestMergeLiveStatusCombinesClaudeAndCodexForAttention(t *testing.T) {
	statuses := map[string]live.Status{
		"both":   {Total: 1, Needing: 1},
		"claude": {Total: 1, Busy: 1},
	}
	mergeLiveStatus(statuses, map[string]live.Status{
		"both":  {Total: 1, Busy: 1},
		"codex": {Total: 1},
	})
	assert.Equal(t, live.Status{Total: 2, Busy: 1, Needing: 1}, statuses["both"])
	assert.Equal(t, live.Status{Total: 1, Busy: 1}, statuses["claude"])
	assert.Equal(t, live.Status{Total: 1}, statuses["codex"])
}

func TestFetcherShowsCodexAndTriggersAttentionOnCompletion(t *testing.T) {
	sessions := makeSessions("alpha")
	l := &lister.MockLister{}
	l.EXPECT().List(lister.ListOptions{}).Return(sessions, nil).Twice()
	tm := &tmux.MockTmux{}
	tm.EXPECT().ListAllPanes().Return([]*model.TmuxPaneAcrossSessions{{SessionName: "alpha", PanePID: 100, PaneCurrentPath: "/tmp/alpha"}}, nil).Twice()
	tm.EXPECT().ListClients().Return(nil, nil).Twice()
	tm.EXPECT().ListAllWindows().Return(nil, nil).Twice()
	reader := &changingCodexReader{statuses: []live.Status{{Total: 1, Busy: 1}, {Total: 1}}}
	fetch := makeClaudeFetcher(&Deps{Lister: l, Tmux: tm, CodexReader: reader, Attention: newAttentionStore(t)}, lister.ListOptions{})

	first, err := fetch(picker.ModeAll)
	require.NoError(t, err)
	assert.Equal(t, picker.LiveBadge{Total: 1, Busy: 1}, first.Decorator.Decorate(sessions.Directory["alpha"]).Live)
	assert.False(t, first.Decorator.Decorate(sessions.Directory["alpha"]).Attention.Triggered)

	second, err := fetch(picker.ModeAll)
	require.NoError(t, err)
	assert.Equal(t, picker.LiveBadge{Total: 1}, second.Decorator.Decorate(sessions.Directory["alpha"]).Live)
	assert.True(t, second.Decorator.Decorate(sessions.Directory["alpha"]).Attention.Triggered)

	l.AssertExpectations(t)
	tm.AssertExpectations(t)
}
