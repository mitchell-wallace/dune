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

# Create workspace and config directories and set permissions
RUN mkdir -p /workspace /home/node/.claude /home/node/.codex /home/node/.gemini && \
  chown -R node:node /workspace /home/node/.claude /home/node/.codex /home/node/.gemini

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

# Copy user home config scaffold from updated/home into /home/node
COPY --chown=node:node home/ /home/node/
COPY --chown=node:node add-omc.sh /home/node/add-omc.sh
RUN chmod +x /home/node/add-omc.sh && \
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
RUN chmod +x /tmp/configure-claude.sh /tmp/configure-codex.sh /tmp/configure-gemini.sh && \
  /tmp/configure-claude.sh && \
  /tmp/configure-codex.sh && \
  /tmp/configure-gemini.sh && \
  rm -f /tmp/configure-claude.sh /tmp/configure-codex.sh /tmp/configure-gemini.sh

# Copy and set up firewall script
COPY init-firewall.sh /usr/local/bin/
USER root
RUN chmod +x /usr/local/bin/init-firewall.sh && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh" > /etc/sudoers.d/node-firewall && \
  chmod 0440 /etc/sudoers.d/node-firewall
USER node
