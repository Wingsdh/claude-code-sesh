package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPickerAttention(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("picker integration tests require python3 for a Unix pseudo-terminal")
	}
	binary := filepath.Join(t.TempDir(), "cc-sesh")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build picker: %v\n%s", err, output)
	}
	t.Run("CodexDisappearanceDoesNotCompleteWork", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, python, "-c", pickerAttentionScenario, binary, "codex-disappears")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("picker scenario: %v\n%s", err, output)
		}
	})
	t.Run("ClaudeCompletionOnlyFlagsItsOwningSession", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, python, "-c", pickerAttentionScenario, binary, "shared-directory")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("picker scenario: %v\n%s", err, output)
		}
	})
}

// Only external process snapshots, tmux responses, and agent files are fixtures.
// Assertions observe the real CLI's rendered session rows, not its state files.
const pickerAttentionScenario = `
import datetime
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import select
import shlex
import struct
import subprocess
import sys
import tempfile
import termios
import time

def text(data):
    return re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", data.decode("utf-8", "replace"))

with tempfile.TemporaryDirectory(prefix="cc-sesh-attention-") as directory:
    root = Path(directory)
    bins = root / "bin"
    bins.mkdir()
    project = root / "project"
    project.mkdir()
    started = datetime.datetime.now() - datetime.timedelta(minutes=2)
    processes = root / "processes"
    shell = "100 1 " + started.ctime() + " /bin/sh\n"
    codex = "200 100 " + started.ctime() + " codex\n"
    processes.write_text(shell + codex)
    ps = bins / "ps"
    ps.write_text("#!/bin/sh\ncat " + shlex.quote(str(processes)) + "\n")
    ps.chmod(0o755)

    sessions = []
    for name in ["owner", "other"]:
        fields = ["0"] * 21
        fields[13] = "$1" if name == "owner" else "$2"
        fields[17] = name
        fields[18] = str(project)
        fields[20] = "1"
        sessions.append("::".join(fields))
    tmux = bins / "tmux"
    tmux.write_text("#!" + sys.executable + "\nimport sys\n"
        + "responses = " + repr({
            "list-sessions": "\n".join(sessions),
            "list-panes": "owner::%1::" + str(project) + "::100::0\nother::%2::" + str(project) + "::300::0",
            "list-clients": "",
            "list-windows": "owner::0::shell::1\nother::0::shell::1",
        }) + "\nprint(responses.get(sys.argv[1], ''))\n")
    tmux.chmod(0o755)
    config = root / "config.toml"
    config.write_text("cache = false\n")
    rollouts = root / ".codex" / "sessions" / started.strftime("%Y/%m/%d")
    rollouts.mkdir(parents=True)
    rollout = rollouts / "rollout-owner.jsonl"
    rollout.write_text(json.dumps({"type": "session_meta", "payload": {
        "cwd": str(project), "timestamp": started.astimezone(datetime.timezone.utc).isoformat()
    }}) + "\n" + json.dumps({"type": "event_msg", "payload": {"type": "task_started"}}) + "\n")
    if sys.argv[2] == "shared-directory":
        processes.write_text(shell + str(os.getpid()) + " 100 " + started.ctime() + " claude\n")
        claude_sessions = root / ".claude" / "sessions"
        claude_sessions.mkdir(parents=True)
        claude_file = claude_sessions / (str(os.getpid()) + ".json")
        claude = {"pid": os.getpid(), "cwd": str(project), "status": "busy", "kind": "interactive"}
        claude_file.write_text(json.dumps(claude))
    env = os.environ.copy()
    env.update({"HOME": str(root), "XDG_STATE_HOME": str(root / "state"),
        "XDG_CONFIG_HOME": str(root / "config"), "XDG_CACHE_HOME": str(root / "cache"),
        "CODEX_HOME": str(root / ".codex"), "TMPDIR": str(root),
        "PATH": str(bins) + os.pathsep + env.get("PATH", ""), "TERM": "xterm-256color", "ENV": ""})

    def picker_rows():
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 90, 0, 0))
        process = subprocess.Popen([sys.argv[1], "--config", str(config), "picker", "-t"],
            stdin=slave, stdout=slave, stderr=slave, env=env)
        os.close(slave)
        data = b""
        rows = {}
        try:
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if not select.select([master], [], [], 0.1)[0]:
                    continue
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                data += chunk
                if b"\x1b[6n" in chunk:
                    os.write(master, b"\x1b[1;1R")
                if b"\x1b[c" in chunk:
                    os.write(master, b"\x1b[?1;2c")
                for line in text(data).splitlines():
                    for name in ["owner", "other"]:
                        if re.search(r"\b" + name + r"\b", line):
                            rows[name] = line
                if len(rows) == 2:
                    return rows
            raise AssertionError("picker did not display fixture sessions: " + text(data))
        finally:
            if process.poll() is None:
                os.write(master, b"\x03")
                try:
                    process.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            os.close(master)

    before = picker_rows()
    assert ("cc" if sys.argv[2] == "shared-directory" else "cx") in before["owner"], before
    assert "●" not in before["owner"], before
    if sys.argv[2] == "shared-directory":
        claude["status"] = "idle"
        claude_file.write_text(json.dumps(claude))
        after = picker_rows()
        assert "●" in after["owner"], "Owner completion was not flagged: " + str(after)
        assert "●" not in after["other"], "Unrelated session got ATTN: " + after["other"]
    else:
        processes.write_text(shell)
        after = picker_rows()
        assert "●" not in after["owner"], "Codex disappeared, but ATTN appeared: " + after["owner"]
`
