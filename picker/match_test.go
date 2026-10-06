package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Wingsdh/cc-sesh/v2/model"
)

func itemsOf(names ...string) sessionItems {
	items := make(sessionItems, len(names))
	for i, n := range names {
		items[i] = sessionItem{name: n, searchName: n}
	}
	return items
}

func matchedNames(items sessionItems, matches []itemMatch) []string {
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = items[m.index].name
	}
	return names
}

// 截图里的真实案例：sahilm/fuzzy 给 bay-translate-harness 17 分、athena 15 分。
func TestFuzzyFind_ContiguousBeatsScatteredSeparatorHits(t *testing.T) {
	items := itemsOf(
		"~/ai-workspace/bay-translate-harness",
		"~/ai-workspace/bayagent-harness",
		"~/Code/athena",
		"~/ai-workspace/vitalbay-harness",
	)
	matches := fuzzyFind("ath", items.searchNames())

	assert.Len(t, matches, 4)
	assert.Equal(t, "~/Code/athena", items[matches[0].index].name)
	assert.Equal(t, []int{7, 8, 9}, matches[0].positions)
}

func TestFuzzyFind_TieBreaksByShorterName(t *testing.T) {
	items := itemsOf("~/Code/athena/docs", "~/Code/athena")
	matches := fuzzyFind("athena", items.searchNames())

	assert.Equal(t, []string{"~/Code/athena", "~/Code/athena/docs"}, matchedNames(items, matches))
}

func TestFuzzyFind_CaseInsensitive(t *testing.T) {
	items := itemsOf("/Applications/Foo.app")

	upper := fuzzyFind("APPS", items.searchNames())
	assert.Len(t, upper, 1)
	assert.Equal(t, []int{1, 2, 3, 12}, upper[0].positions)

	lower := fuzzyFind("foo", itemsOf("~/FOO").searchNames())
	assert.Len(t, lower, 1)
	assert.Equal(t, []int{2, 3, 4}, lower[0].positions)
}

// 重音字符两边都折叠：é 能匹配 café，cafe 也能匹配 café。
func TestFuzzyFind_NormalizesAccents(t *testing.T) {
	items := itemsOf("~/café", "~/Öl")

	assert.Equal(t, []string{"~/café"}, matchedNames(items, fuzzyFind("é", items.searchNames())))
	assert.Equal(t, []string{"~/café"}, matchedNames(items, fuzzyFind("cafe", items.searchNames())))
	assert.Equal(t, []string{"~/Öl"}, matchedNames(items, fuzzyFind("Öl", items.searchNames())))
}

func TestFuzzyFind_NoMatch(t *testing.T) {
	assert.Empty(t, fuzzyFind("xyz", itemsOf("athena", "harness").searchNames()))
}

// 高亮按 rune 下标走，非 ASCII 路径不能错位。
func TestFuzzyFind_PositionsAreRuneIndexes(t *testing.T) {
	items := itemsOf("~/笔记/athena")
	matches := fuzzyFind("ath", items.searchNames())

	assert.Len(t, matches, 1)
	assert.Equal(t, []int{5, 6, 7}, matches[0].positions)
}

func TestApplyFilter_RanksContiguousMatchFirst(t *testing.T) {
	m := newTestModel()
	m.filterInput.SetValue("app")
	m.applyFilter()

	assert.NotEmpty(t, m.filtered)
	assert.Equal(t, "~/code/app", m.filtered[0].item.name)
}

func TestApplyFilter_SeparatorAware_RanksContiguousMatchFirst(t *testing.T) {
	m := newTestModelSeparatorAware()
	m.allItems = buildItems(sessionsOf("~/ai-workspace/bay-translate-harness", "~/Code/athena"), NoDecoration{}, true)
	m.filterInput.SetValue("ath")
	m.applyFilter()

	assert.Len(t, m.filtered, 2)
	assert.Equal(t, "~/Code/athena", m.filtered[0].item.name)
}

func sessionsOf(names ...string) model.SeshSessions {
	dir := model.SeshSessionMap{}
	order := make([]string, len(names))
	for i, n := range names {
		dir[n] = model.SeshSession{Name: n, Src: "zoxide", Path: n}
		order[i] = n
	}
	return model.SeshSessions{OrderedIndex: order, Directory: dir}
}
