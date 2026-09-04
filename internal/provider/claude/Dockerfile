ARG CLAUDE_VERSION=2.1.258

RUN npm install --global @anthropic-ai/claude-code@${CLAUDE_VERSION}

ENV DISABLE_AUTOUPDATER=1 \
    CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1 \
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
