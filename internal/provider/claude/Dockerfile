ARG BASE_IMAGE
FROM ${BASE_IMAGE}

ARG CLAUDE_VERSION=2.1.258
ARG RTK_VERSION=0.47.0

RUN if command -v apk > /dev/null; then \
        apk add --no-cache \
            bash ca-certificates curl file git jq less make nodejs npm \
            openssh-client patch ripgrep tar unzip; \
    elif command -v apt-get > /dev/null; then \
        apt-get update && \
        apt-get install --yes --no-install-recommends \
            bash ca-certificates curl file git jq less make nodejs npm \
            openssh-client patch ripgrep tar unzip && \
        rm -rf /var/lib/apt/lists/*; \
    else \
        echo "unsupported Linux distribution: expected apk or apt-get" >&2; \
        exit 1; \
    fi

RUN npm install --global @anthropic-ai/claude-code@${CLAUDE_VERSION}

RUN set -eu; \
    case "$(uname -m)" in \
        x86_64) rtk_target="x86_64-unknown-linux-musl" ;; \
        aarch64) rtk_target="aarch64-unknown-linux-gnu" ;; \
        *) echo "unsupported architecture for rtk: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    rtk_archive="rtk-${rtk_target}.tar.gz"; \
    rtk_release="https://github.com/rtk-ai/rtk/releases/download/v${RTK_VERSION}"; \
    curl -fsSL "${rtk_release}/${rtk_archive}" -o "/tmp/${rtk_archive}"; \
    curl -fsSL "${rtk_release}/checksums.txt" -o /tmp/rtk-checksums.txt; \
    cd /tmp; \
    rtk_expected="$(grep "[[:space:]]${rtk_archive}$" rtk-checksums.txt | awk '{print $1}')"; \
    rtk_actual="$(sha256sum "${rtk_archive}" | awk '{print $1}')"; \
    test -n "${rtk_expected}"; \
    test "${rtk_actual}" = "${rtk_expected}"; \
    tar -xzf "${rtk_archive}" -C /usr/local/bin rtk; \
    rm "${rtk_archive}" rtk-checksums.txt

ENV DISABLE_AUTOUPDATER=1 \
    RTK_TELEMETRY_DISABLED=1 \
    SHELL=/bin/bash \
    USE_BUILTIN_RIPGREP=0

RUN printf '%s\n' \
        '#!/bin/bash' \
        'set -eu' \
        'mkdir -p "$HOME/.claude"' \
        'if [ ! -f "$HOME/.claude/settings.json" ]; then' \
        '    rtk init --global --hook-only --auto-patch' \
        'fi' \
        'exec claude "$@"' \
        > /usr/local/bin/agbx-claude && \
    chmod 0755 /usr/local/bin/agbx-claude
