FROM debian:12-slim

ARG DEBIAN_FRONTEND=noninteractive
ARG BUILDKIT_INLINE_CACHE=1
ARG TZ=UTC
ARG NODE_MAJOR=22
ARG S6_OVERLAY_VERSION=v3.2.2.0
ARG MAILPIT_VERSION=v1.29.4
ARG EZA_VERSION=v0.23.4
ARG DELTA_VERSION=0.18.2
ARG MISE_VERSION=v2026.3.16
ARG PLAYWRIGHT_VERSION=1.58.2
ARG INSTALL_RALLY=1

ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8
ENV TZ=${TZ}
ENV SHELL=/bin/zsh
ENV EDITOR=nano
ENV VISUAL=nano
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
ENV S6_KEEP_ENV=1
ENV S6_CMD_WAIT_FOR_SERVICES=1
ENV S6_VERBOSITY=1
ENV PATH=/home/agent/.local/bin:/home/agent/.local/share/mise/shims:${PATH}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    bat \
    build-essential \
    ca-certificates \
    curl \
    fd-find \
    fzf \
    gh \
    git \
    gnupg \
    jq \
    locales \
    micro \
    nano \
    postgresql \
    postgresql-client \
    procps \
    redis-server \
    ripgrep \
    sudo \
    tmux \
    tree \
    tzdata \
    unzip \
    vim \
    xz-utils \
    zsh \
  && rm -rf /var/lib/apt/lists/*

RUN sed -i 's/^# *en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen \
  && locale-gen \
  && update-locale LANG=en_US.UTF-8 LANGUAGE=en_US:en LC_ALL=en_US.UTF-8 \
  && ln -snf "/usr/share/zoneinfo/${TZ}" /etc/localtime \
  && echo "${TZ}" > /etc/timezone

RUN install -d -m 0755 /etc/apt/keyrings \
  && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
  && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
    > /etc/apt/sources.list.d/nodesource.list \
  && apt-get update \
  && apt-get install -y --no-install-recommends nodejs \
  && rm -rf /var/lib/apt/lists/*

RUN groupadd --gid 1000 agent \
  && useradd --uid 1000 --gid 1000 --create-home --shell /bin/zsh agent \
  && install -d -m 0755 -o agent -g agent /workspace /persist/agent /opt/home-defaults \
  && install -d -m 0755 -o agent -g agent /var/lib/postgresql/data /var/lib/redis /var/log/redis /var/run/postgresql /tmp/mailpit \
  && echo 'agent ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/agent \
  && chmod 0440 /etc/sudoers.d/agent

RUN arch="$(dpkg --print-architecture)" \
  && case "${arch}" in \
      amd64) s6_arch="x86_64"; mailpit_arch="amd64"; eza_arch="x86_64-unknown-linux-gnu"; delta_pkg_arch="amd64" ;; \
      arm64) s6_arch="aarch64"; mailpit_arch="arm64"; eza_arch="aarch64-unknown-linux-gnu"; delta_pkg_arch="arm64" ;; \
      *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac \
  && curl -fsSL -o /tmp/s6-overlay-noarch.tar.xz \
      "https://github.com/just-containers/s6-overlay/releases/download/${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz" \
  && curl -fsSL -o /tmp/s6-overlay-arch.tar.xz \
      "https://github.com/just-containers/s6-overlay/releases/download/${S6_OVERLAY_VERSION}/s6-overlay-${s6_arch}.tar.xz" \
  && tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz \
  && tar -C / -Jxpf /tmp/s6-overlay-arch.tar.xz \
  && curl -fsSL -o /tmp/mailpit.tar.gz \
      "https://github.com/axllent/mailpit/releases/download/${MAILPIT_VERSION}/mailpit-linux-${mailpit_arch}.tar.gz" \
  && tar -xzf /tmp/mailpit.tar.gz -C /tmp \
  && install -m 0755 /tmp/mailpit /usr/local/bin/mailpit \
  && curl -fsSL -o /tmp/eza.tar.gz \
      "https://github.com/eza-community/eza/releases/download/${EZA_VERSION}/eza_${eza_arch}.tar.gz" \
  && tar -xzf /tmp/eza.tar.gz -C /tmp \
  && install -m 0755 /tmp/eza /usr/local/bin/eza \
  && curl -fsSL -o /tmp/git-delta.deb \
      "https://github.com/dandavison/delta/releases/download/${DELTA_VERSION}/git-delta_${DELTA_VERSION}_${delta_pkg_arch}.deb" \
  && dpkg -i /tmp/git-delta.deb \
  && rm -rf /tmp/s6-overlay-noarch.tar.xz /tmp/s6-overlay-arch.tar.xz /tmp/mailpit.tar.gz /tmp/mailpit /tmp/eza.tar.gz /tmp/eza /tmp/git-delta.deb

RUN ln -sf /usr/bin/batcat /usr/local/bin/bat \
  && ln -sf /usr/bin/fdfind /usr/local/bin/fd

RUN npm install -g \
    @google/gemini-cli \
    @openai/codex \
    opencode-ai \
    playwright@${PLAYWRIGHT_VERSION} \
    pnpm \
    turbo

RUN PLAYWRIGHT_SKIP_BROWSER_GC=1 playwright install --with-deps chromium

COPY container/base/home-defaults/ /opt/home-defaults/
COPY container/base/scripts/configure-agents.sh /usr/local/bin/configure-agents.sh
COPY container/base/scripts/install-rally.sh /usr/local/bin/install-rally.sh
COPY container/base/scripts/setup-persist.sh /usr/local/bin/setup-persist.sh
COPY container/base/s6-overlay/ /etc/s6-overlay/

RUN chmod 0755 /usr/local/bin/configure-agents.sh /usr/local/bin/install-rally.sh /usr/local/bin/setup-persist.sh \
  && find /etc/s6-overlay -type f -exec chmod 0755 {} + \
  && find /opt/home-defaults -type f -name '*.json' -exec chmod 0644 {} + \
  && find /opt/home-defaults -type f -name '*.toml' -exec chmod 0644 {} + \
  && chmod 0644 /opt/home-defaults/.zshrc /opt/home-defaults/.p10k.zsh /opt/home-defaults/.agent-shell-setup.sh

RUN cp /opt/home-defaults/.zshrc /home/agent/.zshrc \
  && cp /opt/home-defaults/.p10k.zsh /home/agent/.p10k.zsh \
  && cp /opt/home-defaults/.agent-shell-setup.sh /home/agent/.agent-shell-setup.sh \
  && install -d -m 0755 -o agent -g agent \
      /home/agent/.claude \
      /home/agent/.codex \
      /home/agent/.gemini \
      /home/agent/.config/opencode \
      /home/agent/.local/share/opencode \
      /home/agent/.config/gh \
      /home/agent/.config/mise \
  && cp /opt/home-defaults/.claude/settings.json /home/agent/.claude/settings.json \
  && cp /opt/home-defaults/.codex/config.toml /home/agent/.codex/config.toml \
  && cp /opt/home-defaults/.codex/mcp-servers.toml /home/agent/.codex/mcp-servers.toml \
  && cp /opt/home-defaults/.gemini/settings.json /home/agent/.gemini/settings.json \
  && cat <<'EOF' > /home/agent/.config/mise/config.toml
[tools]
node = "latest"
go = "latest"
python = "latest"
rust = "latest"
uv = "latest"
EOF

RUN install -d -m 0755 /tmp/mailpit \
  && chown -R agent:agent /home/agent /opt/home-defaults /workspace /persist/agent /tmp/mailpit /var/lib/postgresql /var/lib/redis /var/log/redis /var/run/postgresql

RUN runuser -u agent -- bash -lc 'git clone --depth=1 https://github.com/romkatv/powerlevel10k.git ~/.powerlevel10k' \
  && runuser -u agent -- bash -lc '~/.powerlevel10k/gitstatus/install' \
  && runuser -u agent -- bash -lc 'curl -fsSL https://mise.run | sh' \
  && runuser -u agent -- bash -lc 'mise --version && mise install' \
  && runuser -u agent -- bash -lc 'curl -fsSL https://claude.ai/install.sh | bash' \
  && runuser -u agent -- bash -lc '/usr/local/bin/configure-agents.sh' \
  && if [ "${INSTALL_RALLY}" = "1" ]; then runuser -u agent -- bash -lc '/usr/local/bin/install-rally.sh'; fi

RUN runuser -u agent -- bash -lc 'for bin in python python3 uv go rustc cargo; do ln -sf "$HOME/.local/share/mise/shims/$bin" "$HOME/.local/bin/$bin"; done'

RUN pg_version="$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1 | xargs -n 1 basename)" \
  && runuser -u agent -- bash -lc "/usr/lib/postgresql/${pg_version}/bin/initdb -D /var/lib/postgresql/data --username=agent --auth=trust >/tmp/initdb.log" \
  && printf "%s\n" \
      "listen_addresses = '127.0.0.1'" \
      "port = 5432" \
      "unix_socket_directories = '/var/run/postgresql'" \
      "logging_collector = off" \
      >> /var/lib/postgresql/data/postgresql.conf \
  && printf "%s\n" \
      "local all all trust" \
      "host all all 127.0.0.1/32 trust" \
      "host all all ::1/128 trust" \
      > /var/lib/postgresql/data/pg_hba.conf \
  && runuser -u agent -- bash -lc "/usr/lib/postgresql/${pg_version}/bin/pg_ctl -D /var/lib/postgresql/data -o \"-k /var/run/postgresql\" -w start" \
  && runuser -u agent -- bash -lc "createdb agent" \
  && runuser -u agent -- bash -lc "psql -d postgres -c \"ALTER ROLE agent WITH SUPERUSER LOGIN CREATEDB CREATEROLE;\"" \
  && runuser -u agent -- bash -lc "/usr/lib/postgresql/${pg_version}/bin/pg_ctl -D /var/lib/postgresql/data -m fast -w stop"

WORKDIR /workspace
USER agent
ENTRYPOINT ["/init"]
