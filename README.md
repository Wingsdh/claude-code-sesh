<h1 align="center">cc-sesh</h1>

<p align="center">
  <em>Mission control for Claude Code and Codex in tmux — see live agent status right inside the picker.</em>
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
  <img src="docs/diagrams/cc-sesh-overview.en.png" alt="cc-sesh overview" width="900">
</p>

---

## Why?

Running Claude Code in multiple tmux sessions in parallel is great — until you realize sesh's picker only shows session **names**, not what each Claude is actually **doing**. You end up attaching into every session one by one just to find out: did the refactor finish? Is anyone blocked on a permission prompt? Who's still thinking?

cc-sesh layers Claude Code and Codex status on top of sesh's picker, so you can tell at a glance:

- **how many Claude Code and Codex instances** live in each session and whether they are idle or running; Claude Code also reports input prompts
- a **sticky `ATTN` reminder** when an agent finishes a round of work, so you never miss a "done"
- everything upstream sesh already does — tmux + zoxide + configs + tmuxinator, naming, configuration — kept intact

## Table of Contents

- [Why?](#why)
- [Quick start](#quick-start) — install + tmux config in 30 seconds
- [Commands](#commands)
- [Picker hotkeys](#picker-hotkeys)
- [Shell completion](#shell-completion)
- [Coexistence with upstream sesh](#coexistence-with-upstream-sesh)
- [Configuration](#configuration)
- [Credits & license](#credits--license)

## Quick start

### 1. Install

<details open>
  <summary><b>Homebrew</b> (recommended)</summary>

```sh
brew install Wingsdh/tap/cc-sesh
```

</details>

<details>
  <summary>Go install</summary>

```sh
go install github.com/Wingsdh/cc-sesh/v2@latest
```

Requires Go 1.25+.

</details>

### 2. tmux config

Append the block below to your `~/.tmux.conf` (or `$XDG_CONFIG_HOME/tmux/tmux.conf`):

```tmux
# === cc-sesh ===
# prefix + t — open the picker as a borderless popup
bind-key "t" display-popup -B -E -w 80% -h 70% "cc-sesh picker -i"

# prefix + b — jump back to the previous session
bind-key "b" run-shell "cc-sesh last"

# Don't exit tmux when closing a session (so cc-sesh last has somewhere to go)
set-option -g detach-on-destroy off
```

Or as a one-liner, the whole thing:

```sh
cat >> ~/.tmux.conf <<'EOF'

# === cc-sesh ===
bind-key "t" display-popup -B -E -w 80% -h 70% "cc-sesh picker -i"
bind-key "b" run-shell "cc-sesh last"
set-option -g detach-on-destroy off
EOF
tmux source-file ~/.tmux.conf 2>/dev/null
```

### 3. Use it

Inside tmux, hit **`prefix + t`**. That's it.

> `-i` enables the source icons (requires a [Nerd Font](https://www.nerdfonts.com/)).

## Commands

```sh
cc-sesh picker          # open the built-in picker
cc-sesh list            # list every session source (tmux + config + zoxide + tmuxinator)
cc-sesh connect <name>  # connect to a session (creates it if absent)
cc-sesh last            # jump to the previous session
cc-sesh window          # list / switch / create windows in a session
```

`list / connect / window / pane / clone / root / last` semantics and flags match upstream sesh — see the [upstream usage docs](https://github.com/joshmedeski/sesh#how-to-use) for full details.

## Picker hotkeys

Inside the picker (no `fzf-tmux` wrapper required):

| Hotkey | Action |
|---|---|
| `Ctrl-a` | all sources (default) |
| `Ctrl-t` | tmux sessions only |
| `Ctrl-g` | `sesh.toml` configs only |
| `Ctrl-x` | zoxide history only |
| `Ctrl-f` | walk `$HOME` to depth ≤ 2 |
| `Ctrl-d` | kill the tmux session under cursor |
| `Alt-d`  | dismiss the ATTN flag on the current row (does not kill) |
| `→` | expand the session under cursor (tmux sources only) |
| `←` | collapse the session (from a window row: collapse its session and move the cursor back) |
| `Ctrl-r` | re-capture the preview snapshot |

The preview pane renders only when the terminal is at least 102 columns wide
(list 60 + gap 2 + preview 40); it comes back automatically when you widen the terminal.
On wider terminals the list takes 40% of the width (min 60 columns) and the preview the rest.

## Shell completion

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

## Coexistence with upstream sesh

`sesh` and `cc-sesh` can be installed side-by-side — every name path is renamed:

|             | upstream sesh                       | cc-sesh                                                  |
| ---         | ---                                 | ---                                                      |
| binary      | `sesh`                              | `cc-sesh`                                                |
| Go module   | `github.com/joshmedeski/sesh/v2`    | `github.com/Wingsdh/cc-sesh/v2`                          |
| config dir  | `$XDG_CONFIG_HOME/sesh/`            | `$XDG_CONFIG_HOME/cc-sesh/`                              |
| config file | `sesh.toml`                         | `sesh.toml` (same filename)                              |
| state dir   | —                                   | `$XDG_STATE_HOME/cc-sesh/` *(attention state, fork-only)* |

> The attention state file lives at `$XDG_STATE_HOME/cc-sesh/attention.json` (default `~/.local/state/cc-sesh/attention.json`). Delete it any time to wipe all ATTN flags; the next picker open starts fresh.

## Configuration

The config file lives at `~/.config/cc-sesh/sesh.toml`. **The schema is upstream sesh's** — `[default_session]` / `[[session]]` / `[[wildcard]]` / `[tui]` / `blacklist` / `dir_length` / `sort_order` / `cache`, all of it works as-is — plus the naming options described below.

For configuration syntax and examples, see the [upstream Configuration section](https://github.com/joshmedeski/sesh#configuration).

Apart from [`disable_git_namer`](#folder-only-session-names), cc-sesh adds **no new configuration keys**. Codex CLI instances running inside tmux are found through the pane's process tree; their run/idle status comes from Codex rollout files under `$CODEX_HOME/sessions` (default `~/.codex/sessions`). Codex approval prompts do not currently have a distinct `WAIT` state.

### Session name substitution

`[[name_substitution]]` rules rewrite the path a session name is derived from, so a long or repetitive prefix can be dropped or relabeled. For example, if `~/Workspace/worktrees` is itself a git repo, every directory under it is named `worktrees/<dir>`. This rule drops the prefix:

```toml
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""
```

Connecting to `~/Workspace/worktrees/nodemicro` now creates a session named `nodemicro`.

- **Rules match the `~` path.** Each rule sees the full path with your home directory collapsed to `~` (`~/Workspace/worktrees/nodemicro`, not `/Users/you/Workspace/...`), so write the home part of `find` as `~`. Rules apply to zoxide results, directories you connect to, and wildcard configs. Existing tmux sessions keep their names.
- **The rewritten path is the name.** Whatever is left after the rules becomes the session name in full, and `dir_length` doesn't apply to it. As with any session name, `.` and `:` become `_`, and each run of whitespace becomes a single `_`.
- **Literal by default.** `find` is matched as plain text and every occurrence is replaced, so a `.` in `find` matches only a dot.
- **Regex and capture groups.** Set `regex = true` to treat `find` as a [Go regular expression](https://pkg.go.dev/regexp/syntax). Every match is replaced, and `replace` can reference capture groups as `$1`, `$2`, and so on.
- **Rules run in order.** Rules run top to bottom, and each rule works on the previous rule's output. Rules from files listed in `import` run after the main config's rules, in import order.

Combining the two modes, the literal rule strips the prefix and the regex rule then moves a ticket number to the end:

```toml
[[name_substitution]]
find = "~/Workspace/worktrees/"
replace = ""

[[name_substitution]]
find = "^([0-9]+)_(.*)$"
replace = "$2-$1"
regex = true
```

`~/Workspace/worktrees/12_nodemicro` → `12_nodemicro` → `nodemicro-12`. Swap the two rules and the anchored regex no longer matches, because it sees the full `~/Workspace/...` path.

If no rule changes the path, or the result is left with nothing usable (for example only whitespace), naming falls back to the usual git and directory strategies. That's also what happens if the home directory can't be resolved. A rule with an empty `find`, or a `regex = true` rule whose pattern doesn't compile, is rejected when the config loads, including rules in imported files.

### Folder-only session names

By default, a directory inside a git repo is named `<repo>/<path inside the repo>`, so `~/Workspace/worktrees/nodemicro` becomes `worktrees/nodemicro` when `~/Workspace/worktrees` is a repo. Set `disable_git_namer` to name sessions after their folder and skip the git strategies:

```toml
disable_git_namer = true
```

With this, `~/Workspace/worktrees/nodemicro` is named `nodemicro`, and `cc-sesh root` prints the folder name too, instead of walking up to the git root. `[[name_substitution]]` rules still run first, and `dir_length` still sets how many trailing folders go into the name. Defaults to `false`. This option is specific to cc-sesh; upstream sesh doesn't have it.

## Credits & license

cc-sesh is built on top of [**joshmedeski/sesh**](https://github.com/joshmedeski/sesh). Every piece of session management — tmux / zoxide / tmuxinator integrations, naming strategies, the configuration system, the picker — was designed and polished by Josh Medeski and the upstream contributors over years. Without sesh, there is no cc-sesh. ❤️

MIT, inheriting from upstream [sesh's LICENSE](LICENSE), copyright © 2023 Josh Medeski. Modifications by this fork are released under the same MIT license.

The picker’s AGENT column identifies live tools: orange `cc` for Claude Code, blue (`#0285FF`) `cx` for Codex CLI, and `cc cx` when both run in the same session or window. Session state counts include both tools.
