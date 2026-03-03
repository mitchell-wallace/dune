FROM node:22

ARG TZ
ENV TZ="$TZ"

ARG CLAUDE_CODE_VERSION=latest

# Install basic development tools and iptables/ipset
RUN apt-get update && apt-get install -y --no-install-recommends \
  less \
  git \
  procps \
  sudo \
  fzf \
  zsh \
  man-db \
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
RUN mkdir -p /workspace /home/node/.claude /home/node/.codex /home/node/.gemini /home/node/.config/gh /persist/agent /opt/agent-defaults && \
  chown -R node:node /workspace /home/node/.claude /home/node/.codex /home/node/.gemini /home/node/.config /persist/agent /opt/agent-defaults

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

# Install Gemini CLI
RUN npm install -g @google/gemini-cli

# Install core project tools (beads, mise, etc.)
COPY --chown=node:node install-project-tools.sh /tmp/install-project-tools.sh
USER root
RUN chmod +x /tmp/install-project-tools.sh && \
  INSTALL_USER_TOOLS=0 /tmp/install-project-tools.sh && \
  su - node -c 'INSTALL_SYSTEM_TOOLS=0 /tmp/install-project-tools.sh' && \
  rm -f /tmp/install-project-tools.sh
USER node

# Copy user home config scaffold from updated/home into /home/node
COPY --chown=node:node home/ /home/node/
USER root
COPY setup-agent-persist.sh /usr/local/bin/setup-agent-persist.sh
COPY sand-privileged.sh /usr/local/bin/sand-privileged
COPY sand-poststart.sh /usr/local/bin/sand-poststart.sh
COPY addons-cli.sh /usr/local/bin/addons
COPY addons/ /usr/local/lib/sand/addons/
RUN chmod 0755 /usr/local/bin/setup-agent-persist.sh /usr/local/bin/sand-privileged /usr/local/bin/sand-poststart.sh /usr/local/bin/addons && \
  chown root:root /usr/local/bin/setup-agent-persist.sh /usr/local/bin/sand-privileged /usr/local/bin/sand-poststart.sh /usr/local/bin/addons && \
  chown -R root:root /usr/local/lib/sand && \
  chmod 0755 /usr/local/lib/sand /usr/local/lib/sand/addons && \
  chmod 0444 /usr/local/lib/sand/addons/manifest.tsv && \
  find /usr/local/lib/sand/addons -mindepth 1 -maxdepth 1 -type f ! -name manifest.tsv -exec chmod 0555 {} \;
USER node
RUN \
  mkdir -p /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/gh/config /opt/agent-defaults/git && \
  cp -a /home/node/.claude/. /opt/agent-defaults/claude/ && \
  cp -a /home/node/.codex/. /opt/agent-defaults/codex/ && \
  cp -a /home/node/.gemini/. /opt/agent-defaults/gemini/ && \
  if [ -d /home/node/.config/gh ] && [ -n "$(find /home/node/.config/gh -mindepth 1 -print -quit 2>/dev/null)" ]; then cp -a /home/node/.config/gh/. /opt/agent-defaults/gh/config/; fi && \
  if [ -f /home/node/.gitconfig ] && [ -s /home/node/.gitconfig ]; then cp -a /home/node/.gitconfig /opt/agent-defaults/git/.gitconfig; fi && \
  if [ -f /home/node/.git-credentials ] && [ -s /home/node/.git-credentials ]; then cp -a /home/node/.git-credentials /opt/agent-defaults/git/.git-credentials; fi && \
  /usr/local/bin/setup-agent-persist.sh && \
  touch /home/node/.bashrc /home/node/.zshrc && \
  if ! grep -qxF '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' /home/node/.bashrc; then \
    echo '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' >> /home/node/.bashrc; \
  fi && \
  if ! grep -qxF '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' /home/node/.zshrc; then \
    echo '[ -f ~/.agent-shell-setup.sh ] && source ~/.agent-shell-setup.sh' >> /home/node/.zshrc; \
  fi

# Run per-agent configuration scripts after all CLIs are installed
COPY --chown=node:node configure-claude.sh /tmp/configure-claude.sh
COPY --chown=node:node configure-codex.sh /tmp/configure-codex.sh
COPY --chown=node:node configure-gemini.sh /tmp/configure-gemini.sh
COPY --chown=node:node configure-git-gh.sh /tmp/configure-git-gh.sh
RUN chmod +x /tmp/configure-claude.sh /tmp/configure-codex.sh /tmp/configure-gemini.sh /tmp/configure-git-gh.sh && \
  /tmp/configure-claude.sh && \
  /tmp/configure-codex.sh && \
  /tmp/configure-gemini.sh && \
  /tmp/configure-git-gh.sh && \
  rm -f /tmp/configure-claude.sh /tmp/configure-codex.sh /tmp/configure-gemini.sh /tmp/configure-git-gh.sh

# Refresh defaults after setup scripts so empty persisted volumes can be re-seeded.
RUN rm -rf /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/gh /opt/agent-defaults/git && \
  mkdir -p /opt/agent-defaults/claude /opt/agent-defaults/codex /opt/agent-defaults/gemini /opt/agent-defaults/gh/config /opt/agent-defaults/git && \
  cp -a /persist/agent/claude/. /opt/agent-defaults/claude/ && \
  cp -a /persist/agent/codex/. /opt/agent-defaults/codex/ && \
  cp -a /persist/agent/gemini/. /opt/agent-defaults/gemini/ && \
  cp -a /persist/agent/gh/config/. /opt/agent-defaults/gh/config/ && \
  if [ -f /persist/agent/git/.gitconfig ] && [ -s /persist/agent/git/.gitconfig ]; then cp -a /persist/agent/git/.gitconfig /opt/agent-defaults/git/.gitconfig; fi && \
  if [ -f /persist/agent/git/.git-credentials ] && [ -s /persist/agent/git/.git-credentials ]; then cp -a /persist/agent/git/.git-credentials /opt/agent-defaults/git/.git-credentials; fi

# Copy and set up firewall script
COPY init-firewall.sh /usr/local/bin/
USER root
RUN chmod 0755 /usr/local/bin/init-firewall.sh && \
  chown root:root /usr/local/bin/init-firewall.sh && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/sand-privileged" > /etc/sudoers.d/node-sand-privileged && \
  chmod 0440 /etc/sudoers.d/node-sand-privileged
USER node
