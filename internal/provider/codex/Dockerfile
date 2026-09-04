ARG CODEX_VERSION=0.153.0

RUN npm install --global @openai/codex@${CODEX_VERSION}

RUN printf '%s\n' \
        '#!/bin/bash' \
        'set -eu' \
        'mkdir -p "$HOME/.codex"' \
        'if [ "${1:-}" = "login" ] || [ "${1:-}" = "logout" ]; then' \
        '    exec codex "$@"' \
        'fi' \
        'if ! codex login status > /dev/null 2>&1; then' \
        '    codex login --device-auth' \
        'fi' \
        'exec codex "$@"' \
        > /usr/local/bin/agbx-codex && \
    chmod 0755 /usr/local/bin/agbx-codex
