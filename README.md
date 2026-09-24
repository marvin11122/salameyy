# SALAMEY

<p align="center">
    <a href="https://github.com/wiesnermarven-hue/salameyy"><img width="450" alt="SALAMEY Logo" src="https://github.com/user-attachments/assets/cf8ca3ce-8b02-43f0-9d0f-5a331488da4b" /></a><br />
    <a href="https://github.com/wiesnermarven-hue/salameyy/releases"><img src="https://img.shields.io/github/release/wiesnermarven-hue/salameyy" alt="Latest Release"></a>
    <a href="https://github.com/wiesnermarven-hue/salameyy/actions"><img src="https://github.com/wiesnermarven-hue/salameyy/actions/workflows/build.yml/badge.svg" alt="Build Status"></a>
</p>

<p align="center">Your new coding bestie, now available in your favourite terminal.<br />Your tools, your code, and your workflows, wired into your LLM of choice.</p>
<p align="center">终端里的编程新搭档，<br />无缝接入你的工具、代码与工作流，全面兼容主流 LLM 模型。</p>

<p align="center"><img width="800" alt="SALAMEY Demo" src="https://github.com/user-attachments/assets/58280caf-851b-470a-b6f7-d5c4ea8a1968" /></p>

> [!IMPORTANT]
> `SALAMEY` is the canonical product, module, configuration, and command name.
> The legacy `crush` identifiers remain supported only as compatibility aliases
> for existing installations and integrations.

## Features

- **Multi-Model:** choose from a wide range of LLMs or add your own via OpenAI- or Anthropic-compatible APIs
- **Flexible:** switch LLMs mid-session while preserving context
- **Session-Based:** maintain multiple work sessions and contexts per project
- **LSP-Enhanced:** SALAMEY uses LSPs for additional context, just like you do
- **Extensible:** add capabilities via MCPs (`http`, `stdio`, and `sse`)
- **Works Everywhere:** first-class support in every terminal on macOS, Linux, Windows (PowerShell and WSL), Android, FreeBSD, OpenBSD, and NetBSD
- **Industrial Grade:** built on the Charm ecosystem, powering 25k+ applications, from leading open source projects to business-critical infrastructure

## Installation

Download the latest SALAMEY binaries from the [GitHub releases](https://github.com/wiesnermarven-hue/salameyy/releases), or build the primary `salamey` binary from source:

```bash
git clone https://github.com/wiesnermarven-hue/salameyy.git
cd salameyy
go build -o salamey .
./salamey
```

To run the latest source without creating a local binary:

```bash
go run github.com/wiesnermarven-hue/salameyy@latest
```

The legacy `crush` command and configuration names remain accepted for compatibility, but new installations and documentation use SALAMEY exclusively.

> [!TIP]
> Start with `salamey --help`, then explore providers, skills, MCP servers, and
> project context in the SALAMEY repository.

## Getting Started

The quickest way to get started is to grab an API key for your preferred
provider such as Anthropic, OpenAI, Groq, OpenRouter, or Vercel AI Gateway and
launch the CLI with `salamey`. SALAMEY will prompt you to enter your API key.

That said, you can also set environment variables for preferred providers.

| Environment Variable        | Provider                                           |
| --------------------------- | -------------------------------------------------- |
| `ANTHROPIC_API_KEY`         | Anthropic                                          |
| `OPENAI_API_KEY`            | OpenAI                                             |
| `VERCEL_API_KEY`            | Vercel AI Gateway                                  |
| `GEMINI_API_KEY`            | Google Gemini                                      |
| `SYNTHETIC_API_KEY`         | Synthetic                                          |
| `ZAI_API_KEY`               | Z.ai                                               |
| `MINIMAX_API_KEY`           | MiniMax                                            |
| `HF_TOKEN`                  | Hugging Face Inference                             |
| `CEREBRAS_API_KEY`          | Cerebras                                           |
| `OPENROUTER_API_KEY`        | OpenRouter                                         |
| `IONET_API_KEY`             | io.net                                             |
| `GROQ_API_KEY`              | Groq                                               |
| `VERTEXAI_PROJECT`          | Google Cloud VertexAI (Gemini)                     |
| `VERTEXAI_LOCATION`         | Google Cloud VertexAI (Gemini)                     |
| `AWS_ACCESS_KEY_ID`         | Amazon Bedrock (Claude)                            |
| `AWS_SECRET_ACCESS_KEY`     | Amazon Bedrock (Claude)                            |
| `AWS_REGION`                | Amazon Bedrock (Claude)                            |
| `AWS_PROFILE`               | Amazon Bedrock (Custom Profile)                    |
| `AWS_BEARER_TOKEN_BEDROCK`  | Amazon Bedrock                                     |
| `AZURE_OPENAI_API_ENDPOINT` | Azure OpenAI models                                |
| `AZURE_OPENAI_API_KEY`      | Azure OpenAI models (optional when using Entra ID) |
| `AZURE_OPENAI_API_VERSION`  | Azure OpenAI models                                |

### Subscriptions

If you prefer subscription-based usage, here are some plans that work well in
SALAMEY:

- [Synthetic](https://synthetic.new/pricing)
- [GLM Coding Plan](https://z.ai/subscribe)
- [Kimi Code](https://www.kimi.com/membership/pricing)
- [MiniMax Coding Plan](https://platform.minimax.io/subscribe/coding-plan)

### By the Way

Is there a provider you’d like to see in SALAMEY? Is there an existing model that needs an update?

SALAMEY’s default model listing is managed in [Catwalk](https://github.com/charmbracelet/catwalk), a community-supported, open source repository of SALAMEY-compatible models, and you’re welcome to contribute.

<a href="https://github.com/charmbracelet/catwalk"><img width="174" height="174" alt="Catwalk Badge" src="https://github.com/user-attachments/assets/95b49515-fe82-4409-b10d-5beb0873787d" /></a>

## Configuration

SALAMEY runs great with no configuration. That said, if you do need or want to
customize SALAMEY, configuration can be added either local to the project itself,
or globally, with the following priority:

1. `.salamey.json`
2. `salamey.json`
3. `$HOME/.config/salamey/salamey.json`
4. `.crush.json`
5. `crush.json`
6. `$HOME/.config/crush/crush.json`

SALAMEY uses the `salamey` paths by default. The `crush` filenames and legacy
global location are still accepted as compatibility fallbacks.

Configuration itself is stored as a JSON object:

```json
{
  "this-setting": { "this": "that" },
  "that-setting": ["ceci", "cela"]
}
```

As an additional note, SALAMEY also stores ephemeral data, such as application
state, in its default data location:

```bash
# Unix
$HOME/.local/share/salamey/salamey.json

# Windows
%LOCALAPPDATA%\salamey\salamey.json
```

> [!TIP]
> You can override the user and data config locations by setting:
> * `SALAMEY_GLOBAL_CONFIG`
> * `SALAMEY_GLOBAL_DATA`
> * `CRUSH_GLOBAL_CONFIG`
> * `CRUSH_GLOBAL_DATA`

### LSPs

SALAMEY can use LSPs for additional context to help inform its decisions, just
like you would. LSPs can be added manually like so:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "lsp": {
    "go": {
      "command": "gopls",
      "env": {
        "GOTOOLCHAIN": "go1.26.0"
      }
    },
    "typescript": {
      "command": "typescript-language-server",
      "args": ["--stdio"]
    },
    "nix": {
      "command": "nil"
    }
  }
}
```

### MCPs

SALAMEY also supports Model Context Protocol (MCP) servers through three
transport types: `stdio` for command-line servers, `http` for HTTP endpoints,
and `sse` for Server-Sent Events. Environment variable expansion is supported
using `$(echo $VAR)` syntax.

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "mcp": {
    "filesystem": {
      "type": "stdio",
      "command": "node",
      "args": ["/path/to/mcp-server.js"],
      "timeout": 120,
      "disabled": false,
      "disabled_tools": ["some-tool-name"],
      "env": {
        "NODE_ENV": "production"
      }
    },
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/",
      "timeout": 120,
      "disabled": false,
      "disabled_tools": ["create_issue", "create_pull_request"],
      "headers": {
        "Authorization": "Bearer $GH_PAT"
      }
    },
    "streaming-service": {
      "type": "sse",
      "url": "https://example.com/mcp/sse",
      "timeout": 120,
      "disabled": false,
      "headers": {
        "API-Key": "$(echo $API_KEY)"
      }
    }
  }
}
```

### Ignoring Files

SALAMEY respects `.gitignore` files by default, but you can also create a
`.salameyignore` file to specify additional files and directories that SALAMEY
should ignore. This is useful for excluding files that you want in version
control but don't want SALAMEY to consider when providing context.

The `.salameyignore` file uses the same syntax as `.gitignore` and can be placed
in the root of your project or in subdirectories.

The legacy `.crushignore` filename remains accepted as a compatibility alias.

### Allowing Tools

By default, SALAMEY will ask you for permission before running tool calls. If
you'd like, you can allow tools to be executed without prompting you for
permissions. Use this with care.

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "permissions": {
    "allowed_tools": [
      "view",
      "ls",
      "grep",
      "edit",
      "mcp_context7_get-library-doc"
    ]
  }
}
```

You can also skip all permission prompts entirely by running `salamey --yolo`.
Be very, very careful with this feature.

### Disabling Built-In Tools

If you'd like to prevent SALAMEY from using certain built-in tools entirely, you
can disable them via the `options.disabled_tools` list. Disabled tools are
completely hidden from the agent.

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "disabled_tools": [
      "bash",
      "sourcegraph"
    ]
  }
}
```

To disable tools from MCP servers, see the [MCP config section](#mcps).

### Agent Skills

SALAMEY supports the [Agent Skills](https://agentskills.io) open standard for
extending agent capabilities with reusable skill packages. Skills are folders
containing a `SKILL.md` file with instructions that SALAMEY can discover and
activate on demand.

Skills are discovered from:

- `~/.config/salamey/skills/` on Unix (default, can be overridden with `SALAMEY_SKILLS_DIR`)
- `%LOCALAPPDATA%\salamey\skills\` on Windows (default, can be overridden with `SALAMEY_SKILLS_DIR`)
- Additional paths configured via `options.skills_paths`

Legacy `crush` skills directories and `CRUSH_SKILLS_DIR` remain accepted for
compatibility.

```jsonc
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "skills_paths": [
      "~/.config/salamey/skills", // Windows: "%LOCALAPPDATA%\\salamey\\skills",
      "./project-skills"
    ]
  }
}
```

You can get started with example skills from [anthropics/skills](https://github.com/anthropics/skills):

```bash
# Unix
mkdir -p ~/.config/salamey/skills
cd ~/.config/salamey/skills
git clone https://github.com/anthropics/skills.git _temp
mv _temp/skills/* . && rm -rf _temp
```

```powershell
# Windows (PowerShell)
mkdir -Force "$env:LOCALAPPDATA\salamey\skills"
cd "$env:LOCALAPPDATA\salamey\skills"
git clone https://github.com/anthropics/skills.git _temp
mv _temp/skills/* . ; rm -r -force _temp
```

### Initialization

When you initialize a project, SALAMEY analyzes your codebase and creates
a context file that helps it work more effectively in future sessions.
By default, this file is named `AGENTS.md`, but you can customize the
name and location with the `initialize_as` option:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "initialize_as": "AGENTS.md"
  }
}
```

This is useful if you prefer a different naming convention or want to place the
file in a specific directory (e.g., `SALAMEY.md` or `docs/LLMs.md`). SALAMEY
will fill the file with project-specific context like build commands, code
patterns, and conventions it discovered during initialization.

### Attribution Settings

By default, SALAMEY adds attribution information to Git commits and pull requests
it creates. You can customize this behavior with the `attribution` option:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "attribution": {
      "trailer_style": "co-authored-by",
      "generated_with": true
    }
  }
}
```

- `generated_with`: When true (default), adds `Generated with SALAMEY` to commit
  messages and PR descriptions
- `trailer_style`: Controls the attribution trailer added to commit messages
  (default: `assisted-by`)
	- `assisted-by`: Adds `Assisted-by: [Model Name] via SALAMEY <salamey@users.noreply.github.com>`
	  (includes the model name)
	- `co-authored-by`: Adds `Co-Authored-By: SALAMEY <salamey@users.noreply.github.com>`
	- `none`: No attribution trailer

### Custom Providers

SALAMEY supports custom provider configurations for both OpenAI-compatible and
Anthropic-compatible APIs. For a local `llama.cpp` server, use the
`openai-compat` type with its `/v1` endpoint; the legacy `llama` type is also
accepted as an alias.

> [!NOTE]
> Note that we support two "types" for OpenAI. Make sure to choose the right one
> to ensure the best experience!
> * `openai` should be used when proxying or routing requests through OpenAI.
> * `openai-compat` should be used when using non-OpenAI providers that have OpenAI-compatible APIs.

#### OpenAI-Compatible APIs

Here’s an example configuration for Deepseek, which uses an OpenAI-compatible
API. Don't forget to set `DEEPSEEK_API_KEY` in your environment.

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "providers": {
    "deepseek": {
      "type": "openai-compat",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "$DEEPSEEK_API_KEY",
      "models": [
        {
          "id": "deepseek-chat",
          "name": "Deepseek V3",
          "cost_per_1m_in": 0.27,
          "cost_per_1m_out": 1.1,
          "cost_per_1m_in_cached": 0.07,
          "cost_per_1m_out_cached": 1.1,
          "context_window": 64000,
          "default_max_tokens": 5000
        }
      ]
    }
  }
}
```

#### Anthropic-Compatible APIs

Custom Anthropic-compatible providers follow this format:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "providers": {
    "custom-anthropic": {
      "type": "anthropic",
      "base_url": "https://api.anthropic.com/v1",
      "api_key": "$ANTHROPIC_API_KEY",
      "extra_headers": {
        "anthropic-version": "2023-06-01"
      },
      "models": [
        {
          "id": "claude-sonnet-4-20250514",
          "name": "Claude Sonnet 4",
          "cost_per_1m_in": 3,
          "cost_per_1m_out": 15,
          "cost_per_1m_in_cached": 3.75,
          "cost_per_1m_out_cached": 0.3,
          "context_window": 200000,
          "default_max_tokens": 50000,
          "can_reason": true,
          "supports_attachments": true
        }
      ]
    }
  }
}
```

### Amazon Bedrock

SALAMEY currently supports running Anthropic models through Bedrock, with caching disabled.

- A Bedrock provider will appear once you have AWS configured, i.e. `aws configure`
- SALAMEY also expects the `AWS_REGION` or `AWS_DEFAULT_REGION` to be set
- To use a specific AWS profile set `AWS_PROFILE` in your environment, i.e. `AWS_PROFILE=myprofile salamey`
- Alternatively to `aws configure`, you can also just set `AWS_BEARER_TOKEN_BEDROCK`

### Vertex AI Platform

Vertex AI will appear in the list of available providers when `VERTEXAI_PROJECT` and `VERTEXAI_LOCATION` are set. You will also need to be authenticated:

```bash
gcloud auth application-default login
```

To add specific models to the configuration, configure as such:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "providers": {
    "vertexai": {
      "models": [
        {
          "id": "claude-sonnet-4@20250514",
          "name": "VertexAI Sonnet 4",
          "cost_per_1m_in": 3,
          "cost_per_1m_out": 15,
          "cost_per_1m_in_cached": 3.75,
          "cost_per_1m_out_cached": 0.3,
          "context_window": 200000,
          "default_max_tokens": 50000,
          "can_reason": true,
          "supports_attachments": true
        }
      ]
    }
  }
}
```

### Local Models

Local models can also be configured via OpenAI-compatible API. Here are two common examples:

#### Ollama

```json
{
  "providers": {
    "ollama": {
      "name": "Ollama",
      "base_url": "http://localhost:11434/v1/",
      "type": "openai-compat",
      "models": [
        {
          "name": "Qwen 3 30B",
          "id": "qwen3:30b",
          "context_window": 256000,
          "default_max_tokens": 20000
        }
      ]
    }
  }
}
```

#### LM Studio

```json
{
  "providers": {
    "lmstudio": {
      "name": "LM Studio",
      "base_url": "http://localhost:1234/v1/",
      "type": "openai-compat",
      "models": [
        {
          "name": "Qwen 3 30B",
          "id": "qwen/qwen3-30b-a3b-2507",
          "context_window": 256000,
          "default_max_tokens": 20000
        }
      ]
    }
  }
}
```

## Logging

Sometimes you need to look at logs. Luckily, SALAMEY logs all sorts of
stuff. By default logs are stored in `./.salamey/logs/salamey.log` relative to
the project. Existing `./.crush/logs/crush.log` locations are still read as
compatibility fallbacks.

The CLI also contains some helper commands to make perusing recent logs easier:

```bash
# Print the last 1000 lines
salamey logs

# Print the last 500 lines
salamey logs --tail 500

# Follow logs in real time
salamey logs --follow
```

Want more logging? Run `salamey` with the `--debug` flag, or enable it in the
config:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "debug": true,
    "debug_lsp": true
  }
}
```

## Provider Auto-Updates

By default, SALAMEY automatically checks for the latest and greatest list of
providers and models from [Catwalk](https://github.com/charmbracelet/catwalk),
the open source provider database used by SALAMEY. This means that when new providers and
models are available, or when model metadata changes, SALAMEY automatically
updates your local configuration.

### Disabling automatic provider updates

For those with restricted internet access, or those who prefer to work in
air-gapped environments, this might not be want you want, and this feature can
be disabled.

To disable automatic provider updates, set `disable_provider_auto_update` into
your `salamey.json` config:

```json
{
  "$schema": "https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json",
  "options": {
    "disable_provider_auto_update": true
  }
}
```

Or set the `SALAMEY_DISABLE_PROVIDER_AUTO_UPDATE` environment variable
(the legacy `CRUSH_DISABLE_PROVIDER_AUTO_UPDATE` name also works):

```bash
export SALAMEY_DISABLE_PROVIDER_AUTO_UPDATE=1
```

### Manually updating providers

Manually updating providers is possible with the `salamey update-providers`
command:

```bash
# Update providers remotely from Catwalk.
salamey update-providers

# Update providers from a custom Catwalk base URL.
salamey update-providers https://example.com/

# Update providers from a local file.
salamey update-providers /path/to/local-providers.json

# Reset providers to the embedded version bundled with SALAMEY.
salamey update-providers embedded

# For more info:
salamey update-providers --help
```

## Metrics

SALAMEY records pseudonymous usage metrics (tied to a device-specific hash),
which maintainers rely on to inform development and support priorities. The
metrics include solely usage metadata; prompts and responses are NEVER
collected.

Details on exactly what’s collected are in the source code ([here](https://github.com/wiesnermarven-hue/salameyy/tree/main/internal/event)
and [here](https://github.com/wiesnermarven-hue/salameyy/blob/main/internal/event)).

You can opt out of metrics collection at any time by setting the environment
variable by setting the following in your environment:

```bash
export SALAMEY_DISABLE_METRICS=1
```

Or by setting the following in your config:

```json
{
  "options": {
    "disable_metrics": true
  }
}
```

SALAMEY also respects the `DO_NOT_TRACK` convention which can be enabled via
`export DO_NOT_TRACK=1`.

## Contributing

See the [SALAMEY repository](https://github.com/wiesnermarven-hue/salameyy) for contribution guidelines.

## Community

Open issues and discussions in the [SALAMEY repository](https://github.com/wiesnermarven-hue/salameyy).

## License

[FSL-1.1-MIT](https://github.com/wiesnermarven-hue/salameyy/raw/main/LICENSE.md)
