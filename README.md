# agbx

`agbx` runs coding agents in isolated Docker containers while keeping the
current project directory available as the agent workspace.

Currently, the only supported provider is
[Claude Code](https://docs.anthropic.com/en/docs/claude-code). The project is
intentionally small and configuration-driven, so additional providers and
setup steps can be added without changing the workflow.

## Requirements

- Docker daemon available to the current user
- Go 1.27 or later when building from source

Release builds target Linux, macOS, and Windows on `amd64` and `arm64`. On
Windows, use Docker Desktop configured for Linux containers.

## Installation

Download the archive for your platform from
[GitHub Releases](https://github.com/pixel365/agbx/releases/latest), extract
it, and add the `agbx` binary to your `PATH`.

Or install the latest released version with Go:

```sh
go install github.com/pixel365/agbx@latest
```

Or build from source:

```sh
git clone https://github.com/pixel365/agbx.git
cd agbx
make build
```

The resulting binary is `./bin/agbx`.

## Quick start

From the root of the project you want an agent to work on:

```sh
agbx init
agbx check
agbx prepare claude
agbx run claude
```

`init` opens an interactive wizard that creates `.agbx.yaml`. It can select a
local Docker image, search Docker Hub, or accept an image reference manually.
For non-`latest` Docker Hub tags, the wizard resolves and stores the image
digest when possible.

`prepare` builds a provider image from the configured base image. It is cached
locally and only needs to be run again when the base image or provider setup
changes. Use `agbx prepare claude --force` to rebuild it explicitly.

`run` starts the prepared provider image interactively. The current directory
is mounted read-write at a stable, configuration-specific path below
`/workspace`; provider authentication state is shared between projects under
`${XDG_DATA_HOME:-~/.local/share}/agbx/providers`.

## Configuration

By default, `agbx` reads `.agbx.yaml` or `.agbx.yml` from the current
directory. Pass an explicit configuration file with `--config`:

```sh
agbx --config /path/to/project/.agbx.yaml check
```

The minimal configuration selects the base image used to prepare a provider:

```yaml
version: 1
image:
  name: golang
  tag: 1.27.0-alpine3.24
  digest: sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc
```

Use a digest to pin a non-`latest` image reproducibly. The digest is optional;
an image without it is referenced by name and tag.

### Additional mounts

Additional host paths can be mounted beneath `/agbx` in the container. A mount
may apply to every provider or only to one provider. Sources must already exist.
Relative sources are resolved from the configuration file's directory, and
environment variables use `${NAME}` syntax.

```yaml
version: 1
image:
  name: golang
  tag: 1.27.0-alpine3.24

mounts:
  - source: ./docs
    target: /agbx/docs
    read_only: true
  - source: ${HOME}/.agents/skills
    target: /agbx/.agents/skills
    read_only: true

providers:
  claude:
    mounts:
      - source: ${HOME}/.claude/CLAUDE.md
        target: /agbx/CLAUDE.md
        read_only: true
      - source: ${HOME}/.claude/skills
        target: /agbx/.claude/skills
        read_only: true
```

For Claude Code, any configured additional mount makes `/agbx` available as an
additional working directory. This lets Claude discover mounted instructions
and skills. Provider-specific mounts are combined with shared mounts only for
that provider.

`read_only` defaults to `true`. Mount targets must be absolute paths within
`/agbx`; overlapping targets are rejected.

## Commands

| Command                              | Description                                                      |
| ------------------------------------ | ---------------------------------------------------------------- |
| `agbx init`                          | Interactively create `.agbx.yaml` in the current directory.      |
| `agbx check`                         | Validate the configuration and check Docker daemon availability. |
| `agbx prepare <provider>`            | Build the prepared image for a provider.                         |
| `agbx run <provider> [arguments...]` | Run a prepared provider in the configured container.             |
| `agbx version [-v]`                  | Print version metadata.                                          |

Run `agbx <command> --help` for command-specific options.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines. The
Makefile provides common development commands; run `make help` to list them.

## Security

See [SECURITY.md](.github/SECURITY.md) for vulnerability reporting and release
verification instructions.
