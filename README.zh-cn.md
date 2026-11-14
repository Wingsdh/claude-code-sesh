<h1 align="center">cc-sesh</h1>

<p align="center">
  <em>Claude Code 与 Codex 多会话调度台 —— 在 picker 里看见 tmux session 内的 Agent 实时状态。</em>
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT">
    <img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="MIT License" />
  </a>
  <a href="https://github.com/joshmedeski/sesh">
    <img src="https://img.shields.io/badge/built%20on-sesh-blue.svg" alt="built on sesh" />
  </a>
</p>

<div align="center">

[English](README.md) | [简体中文](README.zh-cn.md)

</div>

<p align="center">
  <img src="docs/diagrams/cc-sesh-overview.zh-CN.png" alt="cc-sesh 示意图" width="900">
</p>

---

## 为什么做这个？

在多个 tmux session 里并行跑 Claude Code 是个好习惯，但 sesh 的 picker 只显示 session **名字**，看不出每个 Claude 当前**在做什么**。结果你得挨个 attach 进去才能知道：那个 refactor 跑完了没？哪个卡在权限弹窗？谁还在思考？

cc-sesh 在 sesh picker 上叠一层 Claude Code 与 Codex 状态，让你一眼看见：

- 每个 session 里**有几个 Claude Code 或 Codex 实例**，分别是空闲还是运行中；Claude Code 还会显示等待输入
- 任何一个 Agent 跑完一轮活时打一个**粘性 ATTN 提醒**，保证你不会错过「完成」
- 上游 sesh 该干的都没动——tmux + zoxide + configs + tmuxinator、命名策略、配置体系全部原样保留

## 目录

- [为什么做这个？](#为什么做这个)
- [快速开始](#快速开始) —— 安装 + tmux 配置一气呵成
- [命令](#命令)
- [Picker 内的 hotkey](#picker-内的-hotkey)
- [Shell 补全](#shell-补全)
- [与上游 sesh 共存](#与上游-sesh-共存)
- [配置](#配置)
- [致谢与 License](#致谢与-license)

## 快速开始

### 1. 安装

<details open>
  <summary><b>Homebrew</b>（推荐）</summary>

```sh
brew install Wingsdh/tap/cc-sesh
```

</details>

<details>
  <summary>Go install</summary>

```sh
go install github.com/Wingsdh/cc-sesh/v2@latest
```

需要 Go 1.25+。

</details>

### 2. tmux 配置

把下面这段贴到你的 `~/.tmux.conf`（或 `$XDG_CONFIG_HOME/tmux/tmux.conf`）：

```tmux
# === cc-sesh ===
# prefix + t —— 无边框弹出 picker
bind-key "t" display-popup -B -E -w 80% -h 70% "cc-sesh picker -i"

# prefix + b —— 跳回上一个 session
bind-key "b" run-shell "cc-sesh last"

# 关闭 session 时不退出 tmux（这样 cc-sesh last 才有 session 可跳）
set-option -g detach-on-destroy off
```

或者一行命令直接追加并 reload：

```sh
cat >> ~/.tmux.conf <<'EOF'

# === cc-sesh ===
bind-key "t" display-popup -B -E -w 80% -h 70% "cc-sesh picker -i"
bind-key "b" run-shell "cc-sesh last"
set-option -g detach-on-destroy off
EOF
tmux source-file ~/.tmux.conf 2>/dev/null
```

### 3. 用起来

在 tmux 里按 **`prefix + t`** 即可。完事。

> `-i` 显示 src icon（需要 [Nerd Font](https://www.nerdfonts.com/)）。

## 命令

```sh
cc-sesh picker          # 打开内置 picker
cc-sesh list            # 列出所有 session 来源（tmux + config + zoxide + tmuxinator）
cc-sesh connect <name>  # connect 到 session（不存在则创建）
cc-sesh last            # 跳到上一个 session
cc-sesh window          # 列出 / 切换 / 新建当前 session 内的 window
```

`list / connect / window / pane / clone / root / last` 等子命令的语义与 flag 与上游 sesh 一致——详见 [上游 README](https://github.com/joshmedeski/sesh#how-to-use)。

## Picker 内的 hotkey

| Hotkey | 行为 |
|---|---|
| `Ctrl-a` | 全部来源（默认） |
| `Ctrl-t` | 仅 tmux session |
| `Ctrl-g` | 仅 `sesh.toml` 配置 |
| `Ctrl-x` | 仅 zoxide 历史 |
| `Ctrl-f` | 在 `$HOME` 下深度 ≤ 2 列目录 |
| `Ctrl-d` | kill 当前光标所指的 tmux session |
| `Alt-d`  | dismiss 当前行的 ATTN 标记（不 kill session） |
| `→` | 展开光标所在 session（仅 tmux 来源） |
| `←` | 折起 session（光标在 window 行时折起其所属 session 并收回光标） |
| `Ctrl-r` | 重取当前预览快照 |

终端宽度 ≥ 102 列（列表 60 + 间距 2 + 预览 40）才渲染预览分栏；拉宽终端会自动恢复。更宽的终端下列表占总宽 40%（下限 60 列），其余给预览。

## Shell 补全

<details>
  <summary>Bash</summary>

```sh
cc-sesh completion bash > cc-sesh-completion.bash
sudo cp cc-sesh-completion.bash /etc/bash_completion.d/
source ~/.bashrc
```

</details>

<details>
  <summary>Zsh</summary>

```sh
cc-sesh completion zsh > _cc-sesh
sudo mkdir -p /usr/local/share/zsh/site-functions
sudo cp _cc-sesh /usr/local/share/zsh/site-functions/
source ~/.zshrc
```

</details>

<details>
  <summary>Fish</summary>

```sh
cc-sesh completion fish > ~/.config/fish/completions/cc-sesh.fish
source ~/.config/fish/config.fish
```

</details>

## 与上游 sesh 共存

`sesh` 和 `cc-sesh` 每个名字路径都改了名，可以同机共存：

|              | 上游 sesh                            | cc-sesh                                                    |
| ---          | ---                                  | ---                                                        |
| 二进制       | `sesh`                               | `cc-sesh`                                                  |
| Go module    | `github.com/joshmedeski/sesh/v2`     | `github.com/Wingsdh/cc-sesh/v2`                            |
| 配置目录     | `$XDG_CONFIG_HOME/sesh/`             | `$XDG_CONFIG_HOME/cc-sesh/`                                |
| 配置文件名   | `sesh.toml`                          | `sesh.toml`（沿用文件名）                                  |
| 状态目录     | —                                    | `$XDG_STATE_HOME/cc-sesh/` *（attention 状态，fork 新增）* |

> attention 状态文件在 `$XDG_STATE_HOME/cc-sesh/attention.json`（默认 `~/.local/state/cc-sesh/attention.json`）。可随时手动删掉清空所有 ATTN 标记，下次 picker 打开会重新积累。

## 配置

配置文件在 `~/.config/cc-sesh/sesh.toml`。**配置 schema 沿用上游 sesh** —— `[default_session]` / `[[session]]` / `[[wildcard]]` / `[tui]` / `blacklist` / `dir_length` / `sort_order` / `cache` 等都原样可用，另外还有下面几节介绍的命名配置。

配置写法请直接看 [上游 README 的 Configuration 一节](https://github.com/joshmedeski/sesh#configuration)。

除了 [`disable_git_namer`](#按文件夹名命名-session)，cc-sesh 本身**没有新增任何配置项**。在 tmux 内运行的 Codex CLI 通过 pane 进程树定位，运行/空闲状态从 `$CODEX_HOME/sessions`（默认 `~/.codex/sessions`）中的 Codex rollout 文件读取。Codex 的审批提示目前没有独立的 `WAIT` 状态。

### session 名替换

`[[name_substitution]]` 规则会改写用来生成 session 名的路径，用来去掉或改写又长又重复的前缀。比如 `~/Workspace/worktrees` 本身是 git 仓库时，它下面的每个目录都会被命名成 `worktrees/<dir>`。下面这条规则去掉这个前缀：

```toml
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""
```

这样连接 `~/Workspace/worktrees/nodemicro` 时，session 名就是 `nodemicro`。

- **规则匹配的是 `~` 开头的路径。** 规则看到的是把 home 目录缩写成 `~` 之后的完整路径（`~/Workspace/worktrees/nodemicro`，而不是 `/Users/you/Workspace/...`），所以 `find` 里涉及 home 目录的部分要写成 `~`。规则对 zoxide 结果、直接连接的目录和 wildcard 配置生效；已经存在的 tmux session 名字不变。
- **改写后的路径就是 session 名。** 规则跑完剩下的内容整体作为 session 名，`dir_length` 对它不生效。和其他 session 名一样，`.` 和 `:` 会变成 `_`，连续的空白会变成一个 `_`。
- **默认按字面匹配。** `find` 当普通文本匹配，所有出现的地方都会被替换，所以 `find` 里的 `.` 只匹配点号本身。
- **正则和捕获组。** 设置 `regex = true` 后，`find` 按 [Go 正则表达式](https://pkg.go.dev/regexp/syntax) 解析。每处匹配都会被替换，`replace` 里可以用 `$1`、`$2` 等引用捕获组。引用后面紧跟字母、数字或下划线时，要加花括号写成 `${1}_x`，不能写 `$1_x`：Go 会把 `$1_x` 当成名为 `1_x` 的捕获组，展开成空串。
- **规则按顺序叠加。** 规则从上到下执行，每条规则处理的是上一条规则的输出。`import` 引入的文件里的规则排在主配置的规则之后，按 import 的顺序执行。

两种模式组合使用：先用字面规则去掉前缀，再用正则规则把编号挪到末尾：

```toml
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""

[[name_substitution]]
find = "^([0-9]+)_(.*)$"
replace = "$2-$1"
regex = true
```

`~/Workspace/worktrees/12_nodemicro` → `12_nodemicro` → `nodemicro-12`。如果把两条规则对调，这条带 `^` 锚点的正则看到的是完整的 `~/Workspace/...` 路径，就匹配不上了。

如果没有任何规则改动路径，或者改写后没剩下能用的内容（比如只剩空白），就回退到原来的 git 和文件夹命名策略。取不到 home 目录时也会这样回退。`find` 为空的规则、或者 `regex = true` 但正则写错的规则，在加载配置时就会报错，import 进来的文件里的规则也一样。

### 按文件夹名命名 session

默认情况下，git 仓库里的目录会被命名成 `<仓库名>/<仓库内路径>`，所以 `~/Workspace/worktrees` 是仓库时，`~/Workspace/worktrees/nodemicro` 会被命名成 `worktrees/nodemicro`。设置 `disable_git_namer` 后会跳过 git 命名策略，直接用文件夹名：

```toml
disable_git_namer = true
```

这样 `~/Workspace/worktrees/nodemicro` 的 session 名就是 `nodemicro`，`cc-sesh root` 输出的也是文件夹名，不再往上找 git 根目录。`[[name_substitution]]` 规则仍然最先执行，`dir_length` 也仍然决定名字取末尾几级文件夹。默认值为 `false`。这个配置项是 cc-sesh 自己加的，上游 sesh 没有。

## 致谢与 License

cc-sesh 站在 [**joshmedeski/sesh**](https://github.com/joshmedeski/sesh) 的肩膀上。所有 session 管理 —— tmux / zoxide / tmuxinator 集成、命名策略、配置系统、picker —— 都是 Josh Medeski 与上游贡献者们多年打磨的成果。**没有 sesh，就没有 cc-sesh。** ❤️

MIT，沿袭上游 [sesh 的 LICENSE](LICENSE)，版权署名 © 2023 Josh Medeski。本仓库新增部分同样以 MIT 发布。

Picker 的 AGENT 列标识运行中的工具：橙色 `cc` 为 Claude Code，蓝色（`#0285FF`）`cx` 为 Codex CLI；同一 session 或 window 同时运行两者时显示 `cc cx`。Session 状态数字为两者合计。
