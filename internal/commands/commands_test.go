package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCommandParsesFrontmatter(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	commandPath := filepath.Join(baseDir, "forge", "overdrive.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(commandPath), 0o755))

	content := `---
title: Overdrive Forge
description: Push the current project into an aggressive upgrade pass.
arguments:
  - id: TARGET
    title: Target
    description: Focus area
    required: true
---
Inspect the project and improve $TARGET.
`
	require.NoError(t, os.WriteFile(commandPath, []byte(content), 0o644))

	command, err := loadCommand(commandPath, baseDir, salameyProjectCommandPrefix, "salamey-project")
	require.NoError(t, err)

	require.Equal(t, "salamey:project:forge:overdrive", command.ID)
	require.Equal(t, "Overdrive Forge", command.Name)
	require.Equal(t, "Push the current project into an aggressive upgrade pass.", command.Description)
	require.Equal(t, "Inspect the project and improve $TARGET.", command.Content)
	require.Equal(t, "salamey-project", command.Scope)
	require.Len(t, command.Arguments, 1)
	require.Equal(t, Argument{
		ID:          "TARGET",
		Title:       "Target",
		Description: "Focus area",
		Required:    true,
	}, command.Arguments[0])
}

func TestLoadCommandFallsBackToInlineArguments(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	commandPath := filepath.Join(baseDir, "pulse.md")
	require.NoError(t, os.WriteFile(commandPath, []byte("Map $AREA then summarize $AREA."), 0o644))

	command, err := loadCommand(commandPath, baseDir, userCommandPrefix, "user")
	require.NoError(t, err)

	require.Equal(t, "user:pulse", command.ID)
	require.Equal(t, "user:pulse", command.Name)
	require.Len(t, command.Arguments, 1)
	require.Equal(t, "AREA", command.Arguments[0].ID)
	require.True(t, command.Arguments[0].Required)
}

func TestLoadCommandParsesRichMetadataAndSteps(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	commandPath := filepath.Join(baseDir, "forge", "pipeline.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(commandPath), 0o755))

	content := `---
title: Overdrive Forge
description: Push the current project into an aggressive upgrade pass for $TARGET.
scope: Workspace
tags:
  - upgrade
  - tui
  - upgrade
aliases:
  - forge
  - overdrive
steps:
  - title: Inspect
    prompt: Inspect $TARGET before changing it.
  - title: Verify
    run: go test ./...
    description: Capture the current safety net first.
arguments:
  - id: TARGET
    title: Target
    description: Focus area
    required: true
---
Then improve $TARGET with the findings.
`
	require.NoError(t, os.WriteFile(commandPath, []byte(content), 0o644))

	command, err := loadCommand(commandPath, baseDir, salameyProjectCommandPrefix, "salamey-project")
	require.NoError(t, err)

	require.Equal(t, "Overdrive Forge", command.Name)
	require.Equal(t, "Push the current project into an aggressive upgrade pass for $TARGET.", command.Description)
	require.Equal(t, "Then improve $TARGET with the findings.", command.Content)
	require.Equal(t, "workspace", command.Scope)
	require.Equal(t, []string{"upgrade", "tui"}, command.Tags)
	require.Equal(t, []string{"forge", "overdrive"}, command.Aliases)
	require.Len(t, command.Steps, 2)
	require.Equal(t, CommandStep{
		Title:  "Inspect",
		Prompt: "Inspect $TARGET before changing it.",
	}, command.Steps[0])
	require.Equal(t, CommandStep{
		Title:       "Verify",
		Description: "Capture the current safety net first.",
		Run:         "go test ./...",
	}, command.Steps[1])
}

func TestLoadCommandExtractsArgumentsFromPipelineSteps(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	commandPath := filepath.Join(baseDir, "audit.md")

	content := `---
steps:
  - Review $AREA for bottlenecks.
  - title: Verify
    run: go test ./$AREA/...
---
`
	require.NoError(t, os.WriteFile(commandPath, []byte(content), 0o644))

	command, err := loadCommand(commandPath, baseDir, userCommandPrefix, "user")
	require.NoError(t, err)

	require.Equal(t, "user:audit", command.Name)
	require.Len(t, command.Arguments, 1)
	require.Equal(t, Argument{
		ID:       "AREA",
		Title:    "AREA",
		Required: true,
	}, command.Arguments[0])
}

func TestCustomCommandBuildPromptForPipeline(t *testing.T) {
	t.Parallel()

	command := CustomCommand{
		Name:        "Overdrive Forge",
		Description: "Aggressively improve $TARGET.",
		Content:     "Ship the highest-leverage upgrade for $TARGET.",
		Scope:       "workspace",
		Tags:        []string{"upgrade", "tui"},
		Aliases:     []string{"forge"},
		Steps: []CommandStep{
			{
				Title:  "Inspect",
				Prompt: "Inspect $TARGET and list the structural bottlenecks.",
			},
			{
				Title:       "Verify",
				Run:         "go test ./...",
				Description: "Capture the current safety net before making changes.",
			},
		},
	}

	prompt := command.BuildPrompt(map[string]string{"TARGET": "the command palette"})

	require.Contains(t, prompt, `Treat "Overdrive Forge" as a multi-step command pipeline.`)
	require.Contains(t, prompt, "Command summary:\nAggressively improve the command palette.")
	require.Contains(t, prompt, "Primary objective:\nShip the highest-leverage upgrade for the command palette.")
	require.Contains(t, prompt, "Metadata:\nscope=workspace\ntags=upgrade, tui\naliases=forge")
	require.Contains(t, prompt, "1. Inspect: Inspect the command palette and list the structural bottlenecks.")
	require.Contains(t, prompt, "2. Verify: Run this local command if tool access is available and it is safe in the current environment:")
	require.Contains(t, prompt, "`go test ./...`")
	require.Contains(t, prompt, "Capture the current safety net before making changes.")
}
