# ROOM

ROOM is a standalone CLI for recursive repo improvement with Codex or Claude Code.

ROOM stands for Repetitively Obsessively Optimize Me.

It runs a self-perpetuating cold-start loop against a git repository: build context from local state, ask the selected agent for one concrete improvement, validate the structured result, optionally commit it, generate the next instruction, and repeat. Think of it as a repeated resonance chamber for repo improvement, built as a sharp local operator tool rather than a platform.

The name is also a small shout-out to Alvin Lucier's [_I Am Sitting in a Room_](https://monoskop.org/Alvin_Lucier).

fka the Token Burner 9000.

## Why

The original shell-script prototype worked until it didn’t. ROOM exists to make that loop reliable:

- one binary
- one JSON contract
- one improvement per iteration
- local state and artifacts for every run
- cold starts instead of fragile conversational continuity
- automatic commit flow when changes are worth keeping
- forced pivots when the loop starts circling

## Prerequisites

ROOM assumes:

- `git` is installed
- either `codex` or `claude` is installed separately
- the selected agent CLI is already authenticated
- the selected agent CLI supports ROOM's headless JSON workflow
- you are running inside a git repository

Check the environment with:

```bash
room doctor
```

## Install

Manual binary download is the primary path.

Release assets include:

- `room_darwin_amd64.tar.gz`
- `room_darwin_arm64.tar.gz`
- `room_linux_amd64.tar.gz`
- `room_linux_arm64.tar.gz`
- `checksums.txt`

### macOS arm64

```bash
curl -L https://github.com/jcpsimmons/room/releases/latest/download/room_darwin_arm64.tar.gz -o room.tar.gz
tar -xzf room.tar.gz
chmod +x room
sudo mv room /usr/local/bin/
```

### macOS amd64

```bash
curl -L https://github.com/jcpsimmons/room/releases/latest/download/room_darwin_amd64.tar.gz -o room.tar.gz
tar -xzf room.tar.gz
chmod +x room
sudo mv room /usr/local/bin/
```

### Linux amd64

```bash
curl -L https://github.com/jcpsimmons/room/releases/latest/download/room_linux_amd64.tar.gz -o room.tar.gz
tar -xzf room.tar.gz
chmod +x room
sudo mv room /usr/local/bin/
```

### Linux arm64

```bash
curl -L https://github.com/jcpsimmons/room/releases/latest/download/room_linux_arm64.tar.gz -o room.tar.gz
tar -xzf room.tar.gz
chmod +x room
sudo mv room /usr/local/bin/
```

Optional install script:

```bash
curl -fsSL https://raw.githubusercontent.com/jcpsimmons/room/main/scripts/install.sh | sh
```

## Quickstart

```bash
room init
room doctor
room inspect
room run --iterations 5
room status
```

To start from a real task instead of the default seed prompt:

```bash
room init --prompt "Build a deterministic changelog generator for this repo."
```

Or pipe a longer prompt through stdin:

```bash
cat prompt.txt | room init --prompt -
```

Common variants:

```bash
room run --iterations 100
room run --until-done
room run --no-commit
room run --allow-dirty
room run --json
```

Provider selection is config-driven. ROOM defaults to Codex. To use Claude Code instead:

```toml
[agent]
provider = "claude"

[claude]
binary = "claude"
permission_mode = "bypassPermissions"
```

## Commands

### `room init`

Creates `.room/` in the current repository and seeds:

- `config.toml`
- `instruction.txt`
- `schema.json`
- `state.json`
- `summaries.log`
- `seen_instructions.txt`
- `runs/`

Use `room init --prompt "..."` to seed `instruction.txt` with your own starting prompt instead of ROOM's default "make this repository materially better" instruction. Pass `--prompt -` to read the prompt from stdin.

### `room run`

Runs the improvement loop. In an interactive terminal, `room run` opens the live TUI by default. Set `ROOM_TUI=0` to force plain progress output instead.

Each iteration:

1. Reads config and local ROOM state.
2. Builds a fresh prompt from the current instruction, recent summaries, prior next-instructions, git status, and recent commits.
3. Calls the configured agent CLI headlessly.
4. Requires JSON matching the ROOM schema.
5. Stores prompt, execution metadata, stdout, stderr, result, and diff artifacts.
6. Optionally commits the change with a consistent prefix.
7. Updates the next instruction, or forces a pivot if the loop is stagnating.

### `room inspect`

Prints the exact prompt body ROOM would send next.

### `room status`

Shows repo path, iteration count, current instruction, last summaries, recent ROOM commits, and dirty state.

### `room doctor`

Checks:

- `git`
- selected provider binary
- Codex version support or Claude CLI capability support
- selected provider auth status
- repo detection
- config parsing
- ROOM state directory health
- write access
- the expectation that provider installation and auth are external

### `room version`

Prints the release version, commit, and build date.

## State And Artifacts

ROOM stores all local orchestration state in `.room/`.

```text
.room/
  config.toml
  instruction.txt
  schema.json
  state.json
  summaries.log
  seen_instructions.txt
  runs/
    0001/
      prompt.txt
      execution.json
      result.json
      stdout.log
      stderr.log
      diff.patch
```

This is local state by design. It makes the loop inspectable, resumable, and debuggable without relying on provider session resume.
ROOM ignores `.room/` in its own dirty checks, diffs, and auto-commits so local state does not contaminate repo improvement runs. If you also want plain `git status` to stay clean, add `.room/` to `.git/info/exclude` or `.gitignore`.

## How ROOM Decides What To Do Next

ROOM owns the framing. The selected agent is asked for exactly one worthwhile improvement and must return:

```json
{
  "summary": "string",
  "next_instruction": "string",
  "status": "continue | pivot | done",
  "commit_message": "string"
}
```

ROOM then applies additional pressure:

- exact duplicate next-instruction detection
- near-duplicate detection with normalized similarity
- repeated subsystem focus detection
- repeated docs/tests/refactor churn detection
- consecutive no-change iteration detection
- consecutive tiny-diff detection

When the loop stalls, ROOM rewrites the next instruction into a forced pivot instead of trusting the repetition.

## Failure Recovery

If a run fails:

- inspect `.room/runs/<n>/`
- read `execution.json`, `stderr.log`, `stdout.log`, and `result.json`
- inspect `diff.patch`
- run `room status`

ROOM preserves raw artifacts so malformed JSON, timeouts, and git issues are diagnosable after the fact.

## Safety And Limitations

ROOM is a power tool. Use it like one.

- v1 is macOS and Linux first
- ROOM does not manage agent install or authentication
- each iteration is a cold start by design
- ROOM requires a clean repo unless `--allow-dirty` is set
- failed iterations stop if they leave the repo in an unsafe dirty state
- ROOM does not do rollback or aggressive reset in v1
- Windows support is not the primary target in v1

## Development

```bash
go test ./...
go build ./cmd/room
```

Release builds are handled through GitHub Actions and Goreleaser.
