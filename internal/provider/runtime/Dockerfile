ARG BASE_IMAGE
FROM ${BASE_IMAGE}

ARG RTK_VERSION=0.47.0
ARG PROVIDER_PACKAGES=""
ARG TOOL_PACKAGES="bash \
    ca-certificates \
    coreutils \
    curl \
    diffutils \
    file \
    findutils \
    git \
    jq \
    less \
    make \
    nodejs \
    npm \
    openssl \
    openssh-client \
    patch \
    procps \
    python3 \
    ripgrep \
    tar \
    unzip \
    util-linux"

RUN if command -v apk > /dev/null; then \
        apk add --no-cache ${TOOL_PACKAGES} ${PROVIDER_PACKAGES}; \
    elif command -v apt-get > /dev/null; then \
        apt-get update && \
        apt-get install --yes --no-install-recommends ${TOOL_PACKAGES} ${PROVIDER_PACKAGES} && \
        rm -rf /var/lib/apt/lists/*; \
    else \
        echo "unsupported Linux distribution: expected apk or apt-get" >&2; \
        exit 1; \
    fi

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

ENV RTK_TELEMETRY_DISABLED=1 \
    SHELL=/bin/bash

RUN printf '%s\n' \
        '#!/bin/sh' \
        'set -eu' \
        'run_as="$1"' \
        'shift' \
        'if [ -n "${AGBX_PROXY_CA_CERTIFICATE:-}" ]; then' \
        '    install -m 0644 "$AGBX_PROXY_CA_CERTIFICATE" /usr/local/share/ca-certificates/agbx-network-audit.crt' \
        '    update-ca-certificates' \
        'fi' \
        'if [ -n "$run_as" ]; then' \
        '    case "$run_as" in' \
        '        *:*) user_id="${run_as%%:*}"; group_id="${run_as#*:}" ;;' \
        '        *) echo "invalid container user: $run_as" >&2; exit 1 ;;' \
        '    esac' \
        '    exec setpriv --reuid "$user_id" --regid "$group_id" --clear-groups -- "$@"' \
        'fi' \
        'exec "$@"' \
        > /usr/local/bin/agbx-run && \
    chmod 0755 /usr/local/bin/agbx-run
