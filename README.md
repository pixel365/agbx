# agbx

`agbx` runs coding agents in isolated Docker containers while keeping the
current project directory available as the agent workspace.

Supported providers are [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
and [Codex](https://learn.chatgpt.com/docs/codex/cli). The project is
intentionally small and configuration-driven, so additional providers and setup
steps can be added without changing the workflow.

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
agbx claude
```

Replace `claude` with `codex` to start Codex instead.

`init` opens an interactive wizard that creates `.agbx.yaml`. It can select a
local Docker image, search Docker Hub, or accept an image reference manually.
For non-`latest` Docker Hub tags, the wizard resolves and stores the image
digest when possible.

The first provider launch builds its image from the configured base image. It
is cached locally; changing the base-image configuration or provider setup
creates a new prepared image automatically. Use `agbx prepare <provider>` to
prebuild it, or add `--force` to rebuild it explicitly. Use `agbx cache list`
to see the current project's prepared images and other AGBX images stored
locally. Images created by earlier AGBX versions remain visible as legacy or
unattributed until they are prepared or launched again.

Use `agbx cache prune` to review historical images that are no longer current
for any known project. It only reports candidates by default; pass `--apply`
to remove them. Unattributed images are never removed automatically.

### Built-in tools

Prepared provider images include a portable command-line toolbox in addition to
the selected base image: `rg` and `fd` for search; `jq` and `yq` for structured
data; Python 3 with `uv`; `git`, `make`, Node.js, npm, and
[RTK](https://github.com/rtk-ai/rtk);
and `shellcheck`, `ip`, `ss`, `lsof`, `nc`, `rsync`, `zip`, `unzip`, and `tar`.
Use Dockerfile fragments when a project needs additional language runtimes,
SDKs, databases, or other specialized tools.

By default, a provider command starts its prepared image interactively. The
current directory is mounted read-write at a stable, configuration-specific path
below `/workspace`; provider authentication state is shared between projects
under `${XDG_DATA_HOME:-~/.local/share}/agbx/providers`.

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

### Dockerfile fragments

Extend a prepared image with Dockerfile fragments. Paths are resolved relative
to the configuration file and may use `${NAME}` environment variables. Shared
fragments run in listed order for every provider; provider fragments run after
them only for that provider.

```yaml
prepare:
  dockerfiles:
    - ${HOME}/.config/agbx/dockerfiles/tools.Dockerfile
    - ./docker/agbx.Dockerfile

providers:
  claude:
    dockerfiles:
      - ./docker/agbx-claude.Dockerfile
```

Each fragment is appended after the runtime and provider Dockerfiles. Use
instructions such as `RUN`, `ENV`, `LABEL`, `USER`, and `WORKDIR`. Do not use
`FROM`, `COPY`, or `ADD`: custom files are not included in the Docker build
context. Changing a fragment creates a new prepared image.

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

For Codex, `/agbx` is passed through `--add-dir`, and Codex runs with its
`workspace-write` sandbox profile. This grants access to the additional
directory alongside the main workspace; Docker still enforces the configured
read-only mount permissions.

On the first `agbx codex`, Codex uses device authentication: open the
displayed link on the host and enter its one-time code. This avoids the browser
redirect callback being sent into the container. Explicit `login` and `logout`
commands remain available through `agbx codex <command>`.

Arguments after a provider name are passed through unchanged, including flags:

```sh
agbx claude --dangerously-skip-permissions
agbx codex --full-auto
```

For a one-off prompt, use each provider's non-interactive mode. Codex also
accepts an initial prompt in its interactive mode:

```sh
agbx claude -p "describe this project"
agbx codex "describe this project"
agbx codex exec "describe this project"
```

Use `agbx help <provider>` for launcher help. Global flags remain before the
provider name, for example `agbx --config /path/to/.agbx.yaml claude`.

`read_only` defaults to `true`. Mount targets must be absolute paths within
`/agbx`; overlapping targets are rejected.

### Network policy

`network.policy` controls HTTP(S) and WebSocket egress through the isolated
network proxy. A policy requires an explicit default action. Rules are exact
hostnames or `*.` subdomain patterns; ports other than `80` and `443` are
always denied. Literal IP destinations are also denied whenever a policy is
active. `deny` wins over `allow` at both configuration levels.

```yaml
network:
  policy:
    default: deny
    allow:
      - github.com
    deny:
      - telemetry.example.com

providers:
  claude:
    network:
      allow:
        - api.anthropic.com
```

Global rules apply to every provider. A provider can add `allow` and `deny`
rules, but cannot change the global default or override a global denial. The
example permits Claude's HTTPS and WSS connections to `api.anthropic.com`, as
well as `github.com`, while denying telemetry. Provider network rules require a
global `network.policy`.

When no policy is configured, networking keeps its existing behavior. Policy
enforcement is performed by the HTTP(S) proxy; it does not claim to be a
general-purpose firewall for protocols that do not use the proxy, DNS, or QUIC.

### Learn network destinations

Use `network learn` to run a provider with temporary audit logging and print a
suggested provider allowlist after the provider exits:

```sh
agbx network learn claude
```

The command intentionally ignores the configured network policy while learning,
so it can observe all HTTP(S) and WebSocket destinations. It applies configured
`network.audit.redact` settings, removes its temporary flow files after reading
them, and never changes `.agbx.yaml`. If the provider exits with an error after
producing flows, the command still prints its suggestion and returns the
provider error.

The generated `providers.<name>.network.allow` snippet must be merged into a
configuration that already has a global policy. For a new restrictive policy,
start with:

```yaml
network:
  policy:
    default: deny
```

### Network audit

To capture outgoing HTTP(S) requests and responses, enable a network audit and
choose a host directory for its logs:

```yaml
network:
  audit:
    log_directory: ${HOME}/.local/state/agbx/network/logs/my-project
    retention:
      max_runs: 20
      max_age: 168h
    redact:
      headers:
        - Authorization
        - X-API-Key
      query_parameters:
        - access_token
        - api_key
```

`agbx` creates an isolated Docker network for the agent and starts an
intercepting proxy as its network peer. Every audited run gets a separate
timestamped subdirectory. The proxy records both an
incremental `flows.mitm` file and `flows.har` when the run ends. Its diagnostic
output is written to `proxy.log`. Its local CA is trusted only inside the agent
container; the private key is stored under
`${XDG_STATE_HOME:-~/.local/state}/agbx/network` with private permissions and
is mounted only into the proxy container.

`retention.max_runs` keeps the newest number of audited runs, while
`retention.max_age` deletes runs older than a Go duration. Either limit can be
omitted; `max_runs: 0` also disables the count limit. Cleanup happens when a
new audit starts and only considers timestamped run directories created by
`agbx`.

`redact.headers` replaces matching request-header values with `[REDACTED]`;
matching is case-insensitive. `redact.query_parameters` replaces matching
request query-parameter values; matching is exact. Both apply after the
request has been sent, before the flow is persisted in `flows.mitm` and
`flows.har`, so they do not change the request sent to the remote service.
Normal proxy flow output is disabled, so `proxy.log` is limited to proxy
diagnostics.

The first audited or policy-controlled run pulls the pinned `mitmproxy` image.
It also prepares a new provider image automatically when the current agbx
runtime requires one. Start with audit alone to inspect the actual destinations
used by a provider before adopting a strict `default: deny` policy.

## Commands

| Command                                        | Description                                                      |
| ---------------------------------------------- | ---------------------------------------------------------------- |
| `agbx init`                                    | Interactively create `.agbx.yaml` in the current directory.      |
| `agbx check [-v]`                              | Validate the configuration and check Docker daemon availability. |
| `agbx prepare <provider> [--force]`            | Prebuild an image; `--force` rebuilds an existing one.           |
| `agbx cache list`                              | List prepared images associated with the current project.        |
| `agbx cache prune [--apply]`                   | Review or remove unused historical prepared images.               |
| `agbx network learn <provider> [arguments...]` | Observe destinations and print a suggested provider allowlist.   |
| `agbx <provider> [arguments...]`               | Start a provider, preparing its image when needed.               |
| `agbx version [-v]`                            | Print version metadata.                                          |

Run `agbx <command> --help` for built-in command options, or
`agbx help <provider>` for launcher help.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines. The
Makefile provides common development commands; run `make help` to list them.

## Security

See [SECURITY.md](.github/SECURITY.md) for vulnerability reporting and release
verification instructions. All containers disable privilege escalation. Normal
provider runs also drop all Linux capabilities. A network-proxied provider starts
as root to install the network proxy's CA, then drops to the configured user and
clears all Linux capability sets before starting the agent.
