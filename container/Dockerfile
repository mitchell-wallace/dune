FROM node:22

ARG TZ=Australia/Melbourne
ENV TZ="$TZ"

ARG CLAUDE_CODE_VERSION=latest
ARG SAND_BUILD_MODE=std
ARG SAND_BUILD_ADDONS=
ARG SAND_PYTHON_VERSION=
ARG SAND_UV_VERSION=
ARG SAND_GO_VERSION=
ARG SAND_RUST_VERSION=

# Install basic development tools and iptables/ipset
RUN apt-get update && apt-get install -y --no-install-recommends \
  less \
  git \
  procps \
  sudo \
  fzf \
  zsh \
  man-db \
  locales \
  unzip \
  gnupg2 \
  gh \
  iptables \
  ipset \
  iproute2 \
  dnsutils \
  aggregate \
  jq \
  nano \
  vim \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN printf 'en_AU.UTF-8 UTF-8\nen_US.UTF-8 UTF-8\n' > /etc/locale.gen && \
  locale-gen && \
  update-locale LANG=en_AU.UTF-8 LANGUAGE=en_AU:en && \
  ln -snf "/usr/share/zoneinfo/${TZ}" /etc/localtime && \
  printf '%s\n' "$TZ" > /etc/timezone

ENV LANG=en_AU.UTF-8
ENV LANGUAGE=en_AU:en

# Ensure default node user has access to /usr/local/share
RUN mkdir -p /usr/local/share/npm-global && \
  chown -R node:node /usr/local/share

ARG USERNAME=node

# Persist bash history.
RUN SNIPPET="export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
  && mkdir /commandhistory \
  && touch /commandhistory/.bash_history \
  && chown -R $USERNAME /commandhistory

# Set `DEVCONTAINER` environment variable to help with orientation
ENV DEVCONTAINER=true

# Create workspace, config directories, and persisted agent storage
RUN mkdir -p /workspace /home/node/.claude /home/node/.codex /home/node/.gemini /home/node/.config/gh /home/node/.config/opencode /home/node/.local/share/opencode /persist/agent /opt/agent-defaults && \
  chown -R node:node /workspace /home/node/.claude /home/node/.codex /home/node/.gemini /home/node/.config /home/node/.local /persist/agent /opt/agent-defaults

WORKDIR /workspace

ARG GIT_DELTA_VERSION=0.18.2
RUN ARCH=$(dpkg --print-architecture) && \
  wget "https://github.com/dandavison/delta/releases/download/${GIT_DELTA_VERSION}/git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  sudo dpkg -i "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  rm "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb"

# Set up non-root user
USER node

# Install global packages
ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin
ENV PATH=/home/node/.local/bin:$PATH
ENV SAND_PYTHON_VERSION="${SAND_PYTHON_VERSION}"
ENV SAND_UV_VERSION="${SAND_UV_VERSION}"
ENV SAND_GO_VERSION="${SAND_GO_VERSION}"
ENV SAND_RUST_VERSION="${SAND_RUST_VERSION}"

# Set the default shell to zsh rather than sh
ENV SHELL=/bin/zsh

# Set the default editor and visual
ENV EDITOR=nano
ENV VISUAL=nano

# Default powerline10k theme
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN sh -c "$(wget -O- https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh)" -- \
  -p git \
  -p fzf \
  -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
  -a "source /usr/share/doc/fzf/examples/completion.zsh" \
  -a "export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
  -x

# Install Claude Code once using official bootstrap installer as the runtime user
RUN set -eux; \
  curl -fsSL https://claude.ai/install.sh | bash -s -- "${CLAUDE_CODE_VERSION}"; \
  echo "DEBUG: whoami=$(whoami) HOME=$HOME PATH=$PATH"; \
  echo "DEBUG: listing /home/node/.local/bin"; \
  ls -la /home/node/.local/bin || true; \
  echo "DEBUG: listing /home/node/.claude"; \
  ls -la /home/node/.claude || true; \
  echo "DEBUG: command -v claude"; \
  command -v claude; \
  echo "DEBUG: claude --version"; \
  claude --version

# Install Codex
RUN npm i -g @openai/codex

# Install core project tools (beads, mise, etc.)
COPY --chown=node:node setup/install-project-tools.sh /tmp/install-project-tools.sh
USER root
RUN chmod +x /tmp/install-project-tools.sh && \
  INSTALL_USER_TOOLS=0 /tmp/install-project-tools.sh && \
  su - node -c 'INSTALL_SYSTEM_TOOLS=0 /tmp/install-project-tools.sh' && \
  rm -f /tmp/install-project-tools.sh
USER node

# Copy user home config scaffold into /home/node
COPY --chown=node:node home/ /home/node/
USER root
COPY runtime/setup-agent-persist.sh /usr/local/bin/setup-agent-persist.sh
COPY runtime/sand-privileged.sh /usr/local/bin/sand-privileged
COPY runtime/sand-entrypoint.sh /usr/local/bin/sand-entrypoint.sh
COPY runtime/sand-poststart.sh /usr/local/bin/sand-poststart.sh
COPY runtime/addons-cli.sh /usr/local/bin/addons
COPY addons/ /usr/local/lib/sand/addons/
RUN chmod 0755 /usr/local/bin/setup-agent-persist.sh /usr/local/bin/sand-privileged /usr/local/bin/sand-entrypoint.sh /usr/local/bin/sand-poststart.sh /usr/local/bin/addons && \
  chown root:root /usr/local/bin/setup-agent-persist.sh /usr/local/bin/sand-privileged /usr/local/bin/sand-entrypoint.sh /usr/local/bin/sand-poststart.sh /usr/local/bin/addons && \
  chown -R root:root /usr/local/lib/sand && \
  chmod 0755 /usr/local/lib/sand /usr/local/lib/sand/addons && \
  chmod 0444 /usr/local/lib/sand/addons/manifest.tsv && \
  find /usr/local/lib/sand/addons -mindepth 1 -maxdepth 1 -type f ! -name manifest.tsv -exec chmod 0555 {} \;

# Install configured addons at build time so network-heavy addons complete before runtime firewall setup.
RUN /bin/bash -lc 'set -euo pipefail; \
  mode="$(printf "%s" "${SAND_BUILD_MODE:-std}" | tr "[:upper:]" "[:lower:]")"; \
  case "$mode" in \
    std|standard) mode="std" ;; \
    lax|yolo|strict) ;; \
    *) echo "Invalid SAND_BUILD_MODE='\''$mode'\''" >&2; exit 1 ;; \
  esac; \
  addons_csv="$(printf "%s" "${SAND_BUILD_ADDONS:-}" | tr "[:upper:]" "[:lower:]" | tr -d "[:space:]")"; \
  if [ -z "$addons_csv" ]; then \
    echo "No build-time addons requested."; \
    exit 0; \
  fi; \
  if [ "$mode" = "strict" ]; then \
    echo "Skipping build-time addon install because mode is strict."; \
    exit 0; \
  fi; \
  known_addons="$(tail -n +2 /usr/local/lib/sand/addons/manifest.tsv | cut -f1 | sed "/^$/d")"; \
  installed_count=0; \
  skipped_unknown_count=0; \
  skipped_invalid_count=0; \
  skipped_duplicate_count=0; \
  seen=","; \
  IFS="," read -r -a requested_addons <<<"$addons_csv"; \
  for addon in "${requested_addons[@]}"; do \
    [ -z "$addon" ] && continue; \
    if [[ ! "$addon" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then \
      echo "WARNING: skipping invalid build-time addon name: $addon" >&2; \
      skipped_invalid_count=$((skipped_invalid_count + 1)); \
      continue; \
    fi; \
    if [[ "$seen" == *",$addon,"* ]]; then \
      skipped_duplicate_count=$((skipped_duplicate_count + 1)); \
      continue; \
    fi; \
    seen="${seen}${addon},"; \
    if ! printf "%s\n" "$known_addons" | grep -Fxq "$addon"; then \
      echo "WARNING: skipping unknown build-time addon: $addon" >&2; \
      skipped_unknown_count=$((skipped_unknown_count + 1)); \
      continue; \
    fi; \
    echo "Installing build-time addon: $addon"; \
    /usr/local/bin/sand-privileged run-addon "$addon"; \
    installed_count=$((installed_count + 1)); \
  done; \
  echo "Build-time addon summary: installed=${installed_count} skipped_unknown=${skipped_unknown_count} skipped_invalid=${skipped_invalid_count} skipped_duplicate=${skipped_duplicate_count}"'

USER node
RUN \
  mkdir -p /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/opencode/config /opt/agent-defaults/opencode/data /opt/agent-defaults/gh/config /opt/agent-defaults/git /opt/agent-defaults/addons && \
  cp -a /home/node/.claude/. /opt/agent-defaults/claude/ && \
  cp -a /home/node/.codex/. /opt/agent-defaults/codex/ && \
  cp -a /home/node/.gemini/. /opt/agent-defaults/gemini/ && \
  cp -a /home/node/.config/opencode/. /opt/agent-defaults/opencode/config/ && \
  cp -a /home/node/.local/share/opencode/. /opt/agent-defaults/opencode/data/ && \
  if [ -d /home/node/.config/gh ] && [ -n "$(find /home/node/.config/gh -mindepth 1 -print -quit 2>/dev/null)" ]; then cp -a /home/node/.config/gh/. /opt/agent-defaults/gh/config/; fi && \
  if [ -f /home/node/.gitconfig ] && [ -s /home/node/.gitconfig ]; then cp -a /home/node/.gitconfig /opt/agent-defaults/git/.gitconfig; fi && \
  if [ -f /home/node/.git-credentials ] && [ -s /home/node/.git-credentials ]; then cp -a /home/node/.git-credentials /opt/agent-defaults/git/.git-credentials; fi && \
  if [ -d /persist/agent/addons ] && [ -n "$(find /persist/agent/addons -mindepth 1 -print -quit 2>/dev/null)" ]; then cp -a /persist/agent/addons/. /opt/agent-defaults/addons/; fi && \
  /usr/local/bin/setup-agent-persist.sh && \
  touch /home/node/.bashrc /home/node/.zshrc && \
  if ! grep -qxF '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' /home/node/.bashrc; then \
    echo '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' >> /home/node/.bashrc; \
  fi && \
  if ! grep -qxF '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' /home/node/.zshrc; then \
    echo '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' >> /home/node/.zshrc; \
  fi

# Run post-install agent configuration after all CLIs are installed
COPY --chown=node:node setup/configure-agents.sh /tmp/configure-agents.sh
RUN chmod +x /tmp/configure-agents.sh && \
  /tmp/configure-agents.sh && \
  rm -f /tmp/configure-agents.sh

# Refresh defaults after setup scripts so empty persisted volumes can be re-seeded.
RUN rm -rf /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/opencode /opt/agent-defaults/gh /opt/agent-defaults/git /opt/agent-defaults/addons && \
  mkdir -p /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/opencode/config /opt/agent-defaults/opencode/data /opt/agent-defaults/gh/config /opt/agent-defaults/git /opt/agent-defaults/addons && \
  cp -a /persist/agent/claude/. /opt/agent-defaults/claude/ && \
  cp -a /persist/agent/codex/. /opt/agent-defaults/codex/ && \
  cp -a /persist/agent/gemini/. /opt/agent-defaults/gemini/ && \
  cp -a /persist/agent/opencode/config/. /opt/agent-defaults/opencode/config/ && \
  cp -a /persist/agent/opencode/data/. /opt/agent-defaults/opencode/data/ && \
  cp -a /persist/agent/gh/config/. /opt/agent-defaults/gh/config/ && \
  if [ -d /persist/agent/addons ] && [ -n "$(find /persist/agent/addons -mindepth 1 -print -quit 2>/dev/null)" ]; then cp -a /persist/agent/addons/. /opt/agent-defaults/addons/; fi && \
  if [ -f /persist/agent/git/.gitconfig ] && [ -s /persist/agent/git/.gitconfig ]; then cp -a /persist/agent/git/.gitconfig /opt/agent-defaults/git/.gitconfig; fi && \
  if [ -f /persist/agent/git/.git-credentials ] && [ -s /persist/agent/git/.git-credentials ]; then cp -a /persist/agent/git/.git-credentials /opt/agent-defaults/git/.git-credentials; fi

# Copy and set up firewall script
COPY runtime/init-firewall.sh /usr/local/bin/init-firewall.sh
USER root
RUN chmod 0755 /usr/local/bin/init-firewall.sh && \
  chown root:root /usr/local/bin/init-firewall.sh && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/sand-privileged" > /etc/sudoers.d/node-sand-privileged && \
  chmod 0440 /etc/sudoers.d/node-sand-privileged
ENTRYPOINT ["/usr/local/bin/sand-entrypoint.sh"]
USER node
