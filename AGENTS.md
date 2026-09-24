# SALAMEY Development Guide

## Project Overview

SALAMEY is a terminal-based AI coding assistant written in Go. It connects to
LLM providers and gives them tools to read, write, and execute code. It supports
multiple providers, LSPs, MCP servers, agent skills, sessions, and extensible
tools.

The canonical module path is `github.com/wiesnermarven-hue/salameyy`.
The canonical command and product name is `salamey`. The legacy `crush` name is
supported only as a compatibility alias.

## Architecture

```
main.go                            CLI entry point
internal/
  app/                             Application wiring
  cmd/                             Cobra commands
  config/                          Configuration, identity, and provider loading
  agent/                           Session agents and provider coordination
  session/                         Session persistence
  message/                         Message model
  db/                              SQLite persistence and migrations
  lsp/                             Language-server clients
  ui/                              Bubble Tea v2 TUI
  permission/                      Tool permission checks
  skills/                          Skill discovery and loading
  shell/                           Cross-platform shell execution
  event/                           Telemetry
  pubsub/                          Internal event bus
  filetracker/                     Session file tracking
  history/                         Prompt history
  ml/                              Optional image and face-detection helpers
```

## Build and Test

The default build is CGO-free:

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
```

Build the canonical binary explicitly:

```bash
go build -o salamey .
go build -o crush ./cmd/compat
```

The optional GoCV face demo requires OpenCV and CGO:

```bash
CGO_ENABLED=1 go build -tags gocv -o ml_face ./cmd/demo/ml_face
```

The project uses `task` when available:

- `task build` builds `salamey` and the `crush` compatibility alias.
- `task test` runs the CGO-free test suite.
- `task test:race` runs tests with CGO and the race detector.
- `task lint` runs the configured linters.
- `task fmt` runs `gofumpt`.
- `task schema` regenerates `schema.json`.

## Configuration Identity

SALAMEY uses `salamey.json`, `.salamey`, and `SALAMEY_*` as its canonical
configuration, data, and environment identifiers. Existing `crush.json`,
`.crush`, `.crushignore`, and `CRUSH_*` values remain accepted as fallbacks.
Running the binary as `crush` selects compatibility mode; all other invocations
default to SALAMEY.

## Code Style

- Use `context.Context` as the first parameter for operations.
- Follow existing package patterns and use `require` for assertions.
- Use `t.TempDir()` for temporary test directories.
- Use snake_case JSON tags and octal file permissions.
- Start log messages with capital letters.
- Do not add secrets, credentials, or generated binaries to the repository.
- Keep comments focused and avoid comments that do not explain non-obvious
  behavior.

## Working on the TUI

Read `internal/ui/AGENTS.md` before changing TUI code. Keep state changes in the
main model, use Bubble Tea commands for side effects, and use ANSI utilities
from `github.com/charmbracelet/x/ansi` for terminal string handling.
