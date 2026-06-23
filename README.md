# bg — background task runner

Run any command as a detached background task, then watch them all — with live logs — in a terminal UI.

```bash
bg -- claude -p "explain this repo"   # launch, returns immediately with a task id
bg                                    # open the TUI: task list + live logs
```

`bg` is built for long-running CLIs (AI agents, builds, scripts) you want to fire off and check on later. Tasks survive your shell closing — there's no daemon to keep alive. For known AI tools (`claude`, `opencode`, …) it also captures the **session and internal reasoning** into the log, dimmed for readability.

---

## Install

Requires Go 1.25+.

```bash
git clone https://github.com/popoffvg/background
cd background
make build          # produces ./bin/bg
```

Put it on your `PATH`:

```bash
cp ./bin/bg ~/.local/bin/   # or anywhere on $PATH
```

Other Make targets: `make test`, `make vet`, `make run`, `make clean`.

---

## Quick start

```bash
# launch a detached task — everything after `--` is the command
bg -- bash -c 'for i in 1 2 3; do echo line $i; sleep 1; done'
# → 20260623-135757-ufvr

bg ls                       # list tasks with status, start time, age
bg logs 20260623-135757-ufvr        # print its output
bg logs 20260623-135757-ufvr -f     # follow (tail -f)
bg                          # open the interactive TUI

bg kill <id>                # terminate a running task
bg rm <id>                  # delete a finished task's directory
```

---

## Commands

| Command | Description |
| --- | --- |
| `bg -- <cmd> [args…]` | Launch `<cmd>` as a detached task. Prints the new task id. Everything after `--` is run verbatim. |
| `bg` / `bg list` | Open the interactive TUI (task list + live logs). |
| `bg ls` | List tasks as a plain table (scriptable). |
| `bg logs <id>` | Print a task's captured output. |
| `bg logs <id> -f` | Follow the output as it grows. |
| `bg kill <id>` | Send SIGTERM (then SIGKILL) to a running task's process group. |
| `bg rm <id>` | Delete a finished task's directory. Use `--force` to remove a running one. |

---

## How it works

`bg` is **daemonless**. When you run `bg -- <cmd>`:

1. It creates a task directory and re-execs itself as a hidden supervisor, detached via `setsid` so it outlives your shell and terminal.
2. The supervisor runs your command, streams combined stdout+stderr to `out.log`, and writes the exit status when it finishes.
3. `bg` returns immediately with the task id.

State lives under `os.UserCacheDir()/bg/tasks/<id>/` (override the root with the `BG_HOME` environment variable). Each task directory holds:

| File | Contents |
| --- | --- |
| `meta.json` | command, args, working dir, pid, start time, chosen renderer |
| `out.log` | captured output (pretty-rendered for adapter tools) |
| `status.json` | exit code, finish time, signaled flag — written only on completion |

Task status is derived live: a task is **running** if its pid is alive, **exited(N)** / **killed** once `status.json` exists, or **unknown** if the pid is gone with no status (a crash).

---

## AI tool adapters

For known AI CLIs, `bg` auto-injects the flags that make the tool emit a structured event stream, then pretty-renders that stream into `out.log` — assistant text, tool calls, and **internal reasoning** (dimmed, faint + italic, prefixed `💭`). You keep typing the simple command; `bg` adds the flags.

```bash
bg -- claude -p "refactor the parser"
# bg actually runs: claude -p "refactor the parser" --output-format stream-json --verbose
# out.log gets a readable transcript with reasoning dimmed; `bg ls` still shows your original command.
```

### Detection

An adapter matches when **both** hold:
- `filepath.Base(command)` equals the adapter's `bin`, and
- at least one of `when_args` appears in your args (empty `when_args` = always match).

### Built-in defaults

| name | bin | when_args | injected flags | renderer |
| --- | --- | --- | --- | --- |
| claude | `claude` | `-p`, `--print` | `--output-format stream-json --verbose` | `claude-stream-json` |
| opencode | `opencode` | `run` | `--format json --thinking` | `opencode-json` |

### Configuration

User adapters live in `$BG_HOME/adapters.yaml` (or `~/.config/bg/adapters.yaml` when `BG_HOME` is unset). A commented starter file is written on first launch. Entries override built-in defaults by `name`; new names extend the set.

Add a tool — for example `pi`:

```yaml
adapters:
  - name: pi
    bin: pi
    when_args: ["-p"]
    inject: ["--output-format", "json"]
    renderer: claude-stream-json   # reuse claude's parser, or "raw" to copy bytes unchanged
```

Available renderers: `claude-stream-json`, `opencode-json`, `raw`. Unknown names fall back to `raw`. With `raw` (or no adapter), combined stdout+stderr go straight to `out.log` unchanged.

---

## `bg ls` output

Columns: `ID | STATUS | STARTED | AGE | COMMAND`.

- **STARTED** — local clock time (`15:04:05` if today, else `Jan 02 15:04`).
- **AGE** — relative (`2m ago`).
- **COMMAND** — your original command (injected adapter flags hidden).

When stdout is a TTY, **STATUS** is colored (the TUI uses the same mapping):

| Color | Meaning |
| --- | --- |
| 🟢 green | exited, code 0 |
| 🔴 red | non-zero exit, or killed |
| 🔵 blue | running |
| ⚪ grey | unknown / dead (pid gone, no status) |

---

## TUI (`bg` or `bg list`)

Two panes: the task list on the left, live logs on the right. The focused pane has a colored border, and keys route to it. The footer shows `[FOLLOW]` state.

**Focus (works in either pane)**

| Key | Action |
| --- | --- |
| `Tab` | toggle focus left ↔ right |
| `h` / `←` | focus the task list |
| `l` / `→` | focus the log pane |
| `q` / `Ctrl+C` | quit |

**Task list focused**

| Key | Action |
| --- | --- |
| `↑` `↓` `j` `k` | move selection |
| `K` | kill selected running task |
| `d` | delete selected (non-running) task |
| `y` | copy the task's log path to the clipboard |
| `x` | transform the task's output (opens `$EDITOR`) |

**Log pane focused**

| Key | Action |
| --- | --- |
| `↑` `↓` `j` `k` | scroll one line |
| `PgUp` / `Ctrl+B` | page up |
| `PgDn` / `Ctrl+F` | page down |
| `Ctrl+U` / `Ctrl+D` | half page up / down |
| `g` / `Home` | jump to top |
| `G` / `End` | jump to bottom (re-engages follow) |
| `/` | search the log (case-insensitive); `Enter` commits, `Esc` cancels |
| `n` / `N` | jump to next / previous match (wraps) |
| `y` | copy the task's log path to the clipboard |
| `x` | transform the task's output (opens `$EDITOR`) |

Search highlights every match (reverse video) and jumps to the first match
at/after the current scroll position; `Esc` (outside the input bar) clears a
committed search. Switching to a different task clears the search.

Mouse wheel scrolls the log; wheel-up disengages follow.

### Transform

Press `x` (in either pane) to chain a new task off the selected one. `bg` opens
`$EDITOR` (fallback `vi`) on a templated temp file. Write a shell command; lines
starting with `#` and blank lines are stripped. On save+quit the command runs as
a **new detached `bg` task** (`sh -c <cmd>`) with the environment variable
`$FILE` set to the source task's absolute `out.log` path. The new task appears in
the list and records the source task id (`meta.json` `sourceId`) for provenance.

```bash
# inside $EDITOR, for a task whose log you want to grep:
grep -i error "$FILE" | sort | uniq -c
```

**Follow mode** — new output auto-scrolls to the bottom only when follow is on *and* the viewport is already at the bottom. Any upward scroll disengages follow so you aren't yanked away; `G` / `End` re-engages it. Logs are read incrementally (tail-style), so large sessions stay responsive.

---

## Project layout

```
main.go                 entry point; intercepts `bg -- …`
cmd/                    cobra commands (launch, list, ls, logs, kill, rm, supervise)
internal/store/         task directory, meta/status I/O, status derivation
internal/runner/        detached supervisor + process control
internal/adapter/       adapter config + stream renderers (defaults.yaml)
internal/tui/           Bubble Tea UI + incremental log tailing
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Lipgloss](https://github.com/charmbracelet/lipgloss), and [Cobra](https://github.com/spf13/cobra).
