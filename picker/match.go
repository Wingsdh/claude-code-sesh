package picker

import (
	"sort"
	"strings"
	"sync"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// 用 fzf 的 FuzzyMatchV2 打分：连续匹配奖励高、间隔越远罚得越多，
// 不会出现 "ath" 把 bay-t…-h… 排在 athena 前面的情况。

var (
	initAlgo sync.Once
	// slab 是 FuzzyMatchV2 的打分矩阵缓冲（约 200KB）；Update 在单 goroutine 跑，可复用。
	slab *util.Slab
)

type itemMatch struct {
	index     int
	score     int
	positions []int // rune 下标，与 highlightMatches 一致
}

// searchNames 取出参与模糊匹配的 searchName，下标与 items 一一对应。
func (s sessionItems) searchNames() []string {
	names := make([]string, len(s))
	for i, it := range s {
		names[i] = it.searchName
	}
	return names
}

// fuzzyFind 对 names 做大小写不敏感的模糊匹配，
// 按分数降序、同分时短名优先、再按原顺序返回。
// 共享包级 slab，非并发安全：只能在 Model.Update 所在的 goroutine 里调用。
func fuzzyFind(pattern string, names []string) []itemMatch {
	// algo 的字符分类表是包级状态，不 Init 全部是零值；
	// path scheme 让 '/' 后的字符拿边界奖励，并把行首视为路径起点。
	initAlgo.Do(func() {
		algo.Init("path")
		slab = util.MakeSlab(100*1024, 2048)
	})

	// normalize=true 时文本里的 é 会折成 e，pattern 也得同样折叠才对得上。
	runes := algo.NormalizeRunes([]rune(strings.ToLower(pattern)))

	var matches []itemMatch
	for i, name := range names {
		chars := util.ToChars([]byte(name))
		res, pos := algo.FuzzyMatchV2(false, true, true, &chars, runes, true, slab)
		if res.Start < 0 {
			continue
		}
		var positions []int
		if pos != nil {
			positions = append(positions, (*pos)...)
			sort.Ints(positions)
		}
		matches = append(matches, itemMatch{index: i, score: res.Score, positions: positions})
	}

	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score > matches[b].score
		}
		return len(names[matches[a].index]) < len(names[matches[b].index])
	})
	return matches
}
