# progress · fzf-fuzzy-ranking

- 模式：adhoc（单 step）
- skip_test：true（实现与测试已在本 run 之前写完，用户选择「保留实现，只补审查」）
- skip_review：false
- tidy：false（不整理、不 commit；用户看完效果再决定是否合入）
- codex MCP：不可用（连接失败），reviewer 走自审
- Makefile：无（项目用 justfile；本机未装 just，直接跑等价 go 命令）
- baseline：跳过（skip_test）；改动前已跑 `go test -race ./...` 全绿
- 起点：a2a12db（改动未提交）

## 需求

**要做什么**：picker 的模糊匹配排序不合理。输入 `ath` 时，`~/ai-workspace/bay-translate-harness`（三个字符分别落在分隔符后）排在 `~/Code/athena`（连续命中）前面。原因是 sahilm/fuzzy 给分隔符后的字符 +20，而连续匹配奖励只有 +5/+15，前导字符惩罚封顶 -15，未匹配字符只扣 -1。

**可观察行为**：
- 连续子串命中排在零散命中前面（`ath` → `~/Code/athena` 第一）
- 同分时短名优先，再按原顺序
- 忽略大小写
- 高亮下标是 rune 下标，非 ASCII 路径不错位
- separator-aware 模式下行为一致（空格可匹配 `-` `_` `/` `\`）

**MUST**：
- 用 fzf 的 `FuzzyMatchV2`（path scheme）替换 sahilm/fuzzy
- `algo.Init("path")` 必须在首次匹配前调用

**NEVER**：
- 不改 attention 项置顶的排序规则
- 不提交、不 push

**文件范围**：`picker/match.go`（新）、`picker/match_test.go`（新）、`picker/tui.go`、`go.mod`、`go.sum`

## Steps

| Step | 状态 | RED | GREEN | REVIEW | 备注 |
|------|------|-----|-------|--------|------|
| 01-fzf-fuzzy-ranking | done | skipped | 第 2 轮通过 (22:42) | R01 revise: P1×1 (22:41)；review_pass self R02 (22:43) | 改 5 文件；R01 修 pattern 未做重音折叠（NormalizeRunes），slab 复用，补大小写/重音测试 |

## 整体回归

final regression 通过 (22:43)：`go test -race ./...` 18 个包全绿，`go vet ./...` 干净。

## Run 完成

- skip_test=true / skip_review=false / tidy=false；codex 不可用，审查 100% 自审
- 未 commit：用户先本地试用再决定是否合入
- 本地安装：`~/go/bin/cc-sesh`（dev）；已 `brew unlink cc-sesh` 让 tmux popup 用上新版，恢复用 `brew link cc-sesh`
- rebase 到 ba4a90c（#7）时解决冲突：#7 新增的 window 名匹配（`matchWindowNames`）同样改用 `fuzzyFind`，`fuzzyFind` 改为接收 `[]string`；rebase 后 `go test -race ./...` 19 个包全绿
