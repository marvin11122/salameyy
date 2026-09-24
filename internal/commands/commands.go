package commands

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wiesnermarven-hue/salameyy/internal/agent/tools/mcp"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/home"
	"gopkg.in/yaml.v3"
)

var namedArgPattern = regexp.MustCompile(`\$([A-Z][A-Z0-9_]*)`)

const (
	salameyUserCommandPrefix    = "salamey:user:"
	salameyProjectCommandPrefix = "salamey:project:"
	userCommandPrefix           = "user:"
	projectCommandPrefix        = "project:"
)

// Argument represents a command argument with its metadata.
type Argument struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// MCPPrompt represents a custom command loaded from an MCP server.
type MCPPrompt struct {
	ID          string
	Title       string
	Description string
	PromptID    string
	ClientID    string
	Arguments   []Argument
}

// CustomCommand represents a user-defined custom command loaded from markdown files.
type CustomCommand struct {
	ID          string
	Name        string
	Description string
	Content     string
	Arguments   []Argument
	Scope       string
	Tags        []string
	Aliases     []string
	Steps       []CommandStep
}

// CommandStep represents one stage of a multi-step custom command pipeline.
type CommandStep struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Prompt      string `yaml:"prompt"`
	Run         string `yaml:"run"`
}

// UnmarshalYAML allows steps to be expressed as either a bare string or a
// structured mapping.
func (s *CommandStep) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		s.Prompt = strings.TrimSpace(value.Value)
		return nil
	case yaml.MappingNode:
		var raw struct {
			Title       string `yaml:"title"`
			Description string `yaml:"description"`
			Prompt      string `yaml:"prompt"`
			Run         string `yaml:"run"`
			Shell       string `yaml:"shell"`
		}
		if err := value.Decode(&raw); err != nil {
			return err
		}
		s.Title = strings.TrimSpace(raw.Title)
		s.Description = strings.TrimSpace(raw.Description)
		s.Prompt = strings.TrimSpace(raw.Prompt)
		s.Run = strings.TrimSpace(raw.Run)
		if s.Run == "" {
			s.Run = strings.TrimSpace(raw.Shell)
		}
		return nil
	default:
		return fmt.Errorf("unsupported command step format")
	}
}

// StepCount returns the number of pipeline steps defined for the command.
func (c CustomCommand) StepCount() int {
	return len(c.Steps)
}

// HasPipeline reports whether the command is defined as a multi-step pipeline.
func (c CustomCommand) HasPipeline() bool {
	return c.StepCount() > 0
}

// BuildPrompt returns the user-visible prompt to execute for the command.
func (c CustomCommand) BuildPrompt(args map[string]string) string {
	body := substituteArgs(c.Content, args)
	if !c.HasPipeline() {
		return body
	}

	sections := []string{
		fmt.Sprintf("Treat %q as a multi-step command pipeline. Complete the steps in order and carry results forward between them.", c.Name),
	}

	if c.Description != "" {
		sections = append(sections, "Command summary:\n"+substituteArgs(c.Description, args))
	}
	if body != "" {
		sections = append(sections, "Primary objective:\n"+body)
	}
	if c.Scope != "" || len(c.Tags) > 0 || len(c.Aliases) > 0 {
		var meta []string
		if c.Scope != "" {
			meta = append(meta, "scope="+c.Scope)
		}
		if len(c.Tags) > 0 {
			meta = append(meta, "tags="+strings.Join(c.Tags, ", "))
		}
		if len(c.Aliases) > 0 {
			meta = append(meta, "aliases="+strings.Join(c.Aliases, ", "))
		}
		sections = append(sections, "Metadata:\n"+strings.Join(meta, "\n"))
	}

	steps := make([]string, 0, len(c.Steps))
	for i, step := range c.Steps {
		if text := substituteArgs(step.executionText(), args); text != "" {
			steps = append(steps, fmt.Sprintf("%d. %s", i+1, text))
		}
	}
	if len(steps) > 0 {
		sections = append(sections, "Pipeline:\n"+strings.Join(steps, "\n\n"))
	}

	sections = append(sections, "Respond by finishing the pipeline end-to-end. If a local command step cannot be run safely, say so and continue with the best available evidence.")
	return strings.Join(sections, "\n\n")
}

type commandSource struct {
	path   string
	prefix string
	scope  string
}

type commandFrontmatter struct {
	Title       string        `yaml:"title"`
	Description string        `yaml:"description"`
	Arguments   []Argument    `yaml:"arguments"`
	Tags        []string      `yaml:"tags"`
	Aliases     []string      `yaml:"aliases"`
	Scope       string        `yaml:"scope"`
	Steps       []CommandStep `yaml:"steps"`
}

type parsedCommand struct {
	name        string
	description string
	body        string
	arguments   []Argument
	scope       string
	tags        []string
	aliases     []string
	steps       []CommandStep
}

// LoadCustomCommands loads custom commands from multiple sources including
// XDG config directory, home directory, and project directory.
func LoadCustomCommands(cfg *config.Config) ([]CustomCommand, error) {
	return loadAll(buildCommandSources(cfg))
}

// LoadMCPPrompts loads custom commands from available MCP servers.
func LoadMCPPrompts() ([]MCPPrompt, error) {
	var commands []MCPPrompt
	for mcpName, prompts := range mcp.Prompts() {
		for _, prompt := range prompts {
			key := mcpName + ":" + prompt.Name
			var args []Argument
			for _, arg := range prompt.Arguments {
				title := arg.Title
				if title == "" {
					title = arg.Name
				}
				args = append(args, Argument{
					ID:          arg.Name,
					Title:       title,
					Description: arg.Description,
					Required:    arg.Required,
				})
			}
			commands = append(commands, MCPPrompt{
				ID:          key,
				Title:       prompt.Title,
				Description: prompt.Description,
				PromptID:    prompt.Name,
				ClientID:    mcpName,
				Arguments:   args,
			})
		}
	}
	return commands, nil
}

func buildCommandSources(cfg *config.Config) []commandSource {
	var sources []commandSource

	// XDG config directory
	if dir := getXDGCommandsDir("salamey"); dir != "" {
		sources = append(sources, commandSource{
			path:   dir,
			prefix: salameyUserCommandPrefix,
			scope:  "salamey-user",
		})
	}
	if dir := getXDGCommandsDir("crush"); dir != "" {
		sources = append(sources, commandSource{
			path:   dir,
			prefix: userCommandPrefix,
			scope:  "user",
		})
	}

	// Home directory
	if homeDir := home.Dir(); homeDir != "" {
		sources = append(sources, commandSource{
			path:   filepath.Join(homeDir, ".salamey", "commands"),
			prefix: salameyUserCommandPrefix,
			scope:  "salamey-user",
		})
		sources = append(sources, commandSource{
			path:   filepath.Join(homeDir, ".crush", "commands"),
			prefix: userCommandPrefix,
			scope:  "user",
		})
	}

	// Project-local SALAMEY directory.
	sources = append(sources, commandSource{
		path:   filepath.Join(cfg.WorkingDir(), ".salamey", "commands"),
		prefix: salameyProjectCommandPrefix,
		scope:  "salamey-project",
	})

	// Project directory
	sources = append(sources, commandSource{
		path:   filepath.Join(cfg.Options.DataDirectory, "commands"),
		prefix: projectCommandPrefix,
		scope:  "project",
	})

	return sources
}

func loadAll(sources []commandSource) ([]CustomCommand, error) {
	var commands []CustomCommand

	for _, source := range sources {
		if cmds, err := loadFromSource(source); err == nil {
			commands = append(commands, cmds...)
		}
	}

	return commands, nil
}

func loadFromSource(source commandSource) ([]CustomCommand, error) {
	if err := ensureDir(source.path); err != nil {
		return nil, err
	}

	var commands []CustomCommand

	err := filepath.WalkDir(source.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isMarkdownFile(d.Name()) {
			return err
		}

		cmd, err := loadCommand(path, source.path, source.prefix, source.scope)
		if err != nil {
			return nil // Skip invalid files
		}

		commands = append(commands, cmd)
		return nil
	})

	return commands, err
}

func loadCommand(path, baseDir, prefix, scope string) (CustomCommand, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return CustomCommand{}, err
	}

	id := buildCommandID(path, baseDir, prefix)
	parsed := parseCommandContent(id, string(content), scope)

	return CustomCommand{
		ID:          id,
		Name:        parsed.name,
		Description: parsed.description,
		Content:     parsed.body,
		Arguments:   parsed.arguments,
		Scope:       parsed.scope,
		Tags:        parsed.tags,
		Aliases:     parsed.aliases,
		Steps:       parsed.steps,
	}, nil
}

func parseCommandContent(id, content, defaultScope string) parsedCommand {
	body := content
	var meta commandFrontmatter

	if frontmatter, stripped, ok := splitFrontmatter(content); ok {
		if err := yaml.Unmarshal([]byte(frontmatter), &meta); err == nil {
			body = stripped
		}
	}

	body = strings.TrimSpace(body)
	name := strings.TrimSpace(meta.Title)
	if name == "" {
		name = id
	}

	args := normalizeArguments(meta.Arguments)
	if len(args) == 0 {
		args = extractArgNames(commandArgumentSource(body, meta.Steps))
	}

	return parsedCommand{
		name:        name,
		description: strings.TrimSpace(meta.Description),
		body:        body,
		arguments:   args,
		scope:       normalizeScope(defaultScope, meta.Scope),
		tags:        normalizeStringList(meta.Tags),
		aliases:     normalizeStringList(meta.Aliases),
		steps:       normalizeSteps(meta.Steps),
	}
}

func splitFrontmatter(content string) (string, string, bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", content, false
	}

	rest := normalized[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", content, false
	}

	return rest[:idx], strings.TrimLeft(rest[idx+len("\n---\n"):], "\n"), true
}

func normalizeArguments(arguments []Argument) []Argument {
	if len(arguments) == 0 {
		return nil
	}

	result := make([]Argument, 0, len(arguments))
	for _, arg := range arguments {
		if arg.ID == "" {
			continue
		}
		if arg.Title == "" {
			arg.Title = arg.ID
		}
		result = append(result, arg)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeScope(defaultScope, override string) string {
	override = strings.TrimSpace(override)
	if override == "" {
		return defaultScope
	}
	return strings.ToLower(override)
}

func normalizeSteps(steps []CommandStep) []CommandStep {
	if len(steps) == 0 {
		return nil
	}

	result := make([]CommandStep, 0, len(steps))
	for _, step := range steps {
		step.Title = strings.TrimSpace(step.Title)
		step.Description = strings.TrimSpace(step.Description)
		step.Prompt = strings.TrimSpace(step.Prompt)
		step.Run = strings.TrimSpace(step.Run)
		if step.executionText() == "" {
			continue
		}
		result = append(result, step)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func commandArgumentSource(body string, steps []CommandStep) string {
	parts := []string{body}
	for _, step := range steps {
		parts = append(parts, step.Title, step.Description, step.Prompt, step.Run)
	}
	return strings.Join(parts, "\n")
}

func extractArgNames(content string) []Argument {
	matches := namedArgPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var args []Argument

	for _, match := range matches {
		arg := match[1]
		if !seen[arg] {
			seen[arg] = true
			// for normal custom commands, all args are required
			args = append(args, Argument{ID: arg, Title: arg, Required: true})
		}
	}

	return args
}

func (s CommandStep) executionText() string {
	switch {
	case s.Run != "":
		prefix := "Run this local command if tool access is available and it is safe in the current environment"
		if s.Title != "" {
			prefix = s.Title + ": " + prefix
		}
		text := prefix + ":\n`" + s.Run + "`"
		if s.Description != "" {
			text += "\n" + s.Description
		}
		return text
	case s.Prompt != "":
		text := s.Prompt
		if s.Title != "" {
			text = s.Title + ": " + text
		}
		if s.Description != "" {
			text += "\n" + s.Description
		}
		return text
	default:
		return ""
	}
}

func substituteArgs(content string, args map[string]string) string {
	if content == "" || len(args) == 0 {
		return content
	}

	return namedArgPattern.ReplaceAllStringFunc(content, func(match string) string {
		if value, ok := args[strings.TrimPrefix(match, "$")]; ok {
			return value
		}
		return match
	})
}

func buildCommandID(path, baseDir, prefix string) string {
	relPath, _ := filepath.Rel(baseDir, path)
	parts := strings.Split(relPath, string(filepath.Separator))

	// Remove .md extension from last part
	if len(parts) > 0 {
		lastIdx := len(parts) - 1
		parts[lastIdx] = strings.TrimSuffix(parts[lastIdx], filepath.Ext(parts[lastIdx]))
	}

	return prefix + strings.Join(parts, ":")
}

func getXDGCommandsDir(appName string) string {
	xdgHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgHome == "" {
		if home := home.Dir(); home != "" {
			xdgHome = filepath.Join(home, ".config")
		}
	}
	if xdgHome != "" {
		return filepath.Join(xdgHome, appName, "commands")
	}
	return ""
}

func ensureDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0o755)
	}
	return nil
}

func isMarkdownFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}

func GetMCPPrompt(cfg *config.Config, clientID, promptID string, args map[string]string) (string, error) {
	// TODO: we should pass the context down
	result, err := mcp.GetPromptMessages(context.Background(), cfg, clientID, promptID, args)
	if err != nil {
		return "", err
	}
	return strings.Join(result, " "), nil
}
