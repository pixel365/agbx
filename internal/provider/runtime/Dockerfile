ARG BASE_IMAGE
FROM ${BASE_IMAGE}

ARG RTK_VERSION=0.47.0
ARG UV_VERSION=0.12.11
ARG YQ_VERSION=4.53.3
ARG PROVIDER_PACKAGES=""
ARG TOOL_PACKAGES="bash \
    ca-certificates \
    coreutils \
    curl \
    diffutils \
    file \
    findutils \
    git \
    iproute2 \
    jq \
    less \
    lsof \
    make \
    netcat-openbsd \
    nodejs \
    npm \
    openssl \
    openssh-client \
    patch \
    procps \
    python3 \
    ripgrep \
    rsync \
    shellcheck \
    tar \
    unzip \
    util-linux \
    zip"

RUN if command -v apk > /dev/null; then \
        apk add --no-cache ${TOOL_PACKAGES} fd ${PROVIDER_PACKAGES}; \
    elif command -v apt-get > /dev/null; then \
        apt-get update && \
        apt-get install --yes --no-install-recommends ${TOOL_PACKAGES} fd-find ${PROVIDER_PACKAGES} && \
        rm -rf /var/lib/apt/lists/*; \
    else \
        echo "unsupported Linux distribution: expected apk or apt-get" >&2; \
        exit 1; \
    fi && \
    if ! command -v fd > /dev/null && command -v fdfind > /dev/null; then \
        ln -s "$(command -v fdfind)" /usr/local/bin/fd; \
    fi

RUN set -eu; \
    case "$(uname -m)" in \
        x86_64) yq_target="amd64" ;; \
        aarch64) yq_target="arm64" ;; \
        *) echo "unsupported architecture for yq: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    yq_binary="yq_linux_${yq_target}"; \
    yq_release="https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}"; \
    curl -fsSL "${yq_release}/${yq_binary}" -o "/tmp/${yq_binary}"; \
    curl -fsSL "${yq_release}/checksums" -o /tmp/yq-checksums; \
    curl -fsSL "${yq_release}/checksums_hashes_order" -o /tmp/yq-checksum-algorithms; \
    yq_checksum_column="$(grep -n '^SHA-256$' /tmp/yq-checksum-algorithms | cut -d: -f1)"; \
    yq_checksum_column="$((yq_checksum_column + 1))"; \
    yq_expected="$(grep -m 1 "^${yq_binary}[[:space:]]" /tmp/yq-checksums | awk -v field="${yq_checksum_column}" '{print $field}')"; \
    yq_actual="$(sha256sum "/tmp/${yq_binary}" | awk '{print $1}')"; \
    test -n "${yq_expected}"; \
    test "${yq_actual}" = "${yq_expected}"; \
    install -m 0755 "/tmp/${yq_binary}" /usr/local/bin/yq; \
    rm "/tmp/${yq_binary}" /tmp/yq-checksums /tmp/yq-checksum-algorithms

RUN set -eu; \
    case "$(uname -m)" in \
        x86_64) uv_architecture="x86_64" ;; \
        aarch64) uv_architecture="aarch64" ;; \
        *) echo "unsupported architecture for uv: $(uname -m)" >&2; exit 1 ;; \
    esac; \
    if command -v apk > /dev/null; then \
        uv_libc="musl"; \
    else \
        uv_libc="gnu"; \
    fi; \
    uv_target="${uv_architecture}-unknown-linux-${uv_libc}"; \
    uv_archive="uv-${uv_target}.tar.gz"; \
    uv_release="https://github.com/astral-sh/uv/releases/download/${UV_VERSION}"; \
    curl -fsSL "${uv_release}/${uv_archive}" -o "/tmp/${uv_archive}"; \
    curl -fsSL "${uv_release}/${uv_archive}.sha256" -o "/tmp/${uv_archive}.sha256"; \
    cd /tmp; \
    sha256sum -c "${uv_archive}.sha256"; \
    tar -xzf "${uv_archive}"; \
    test -f "uv-${uv_target}/uv"; \
    test -f "uv-${uv_target}/uvx"; \
    install -m 0755 "uv-${uv_target}/uv" /usr/local/bin/uv; \
    install -m 0755 "uv-${uv_target}/uvx" /usr/local/bin/uvx; \
    rm -rf "${uv_archive}" "${uv_archive}.sha256" "uv-${uv_target}"

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
        '    exec setpriv --bounding-set=-all --inh-caps=-all --ambient-caps=-all --reuid "$user_id" --regid "$group_id" --clear-groups -- "$@"' \
        'fi' \
        'exec "$@"' \
        > /usr/local/bin/agbx-run && \
    chmod 0755 /usr/local/bin/agbx-run
