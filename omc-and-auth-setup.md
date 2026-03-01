You can’t realistically do “interactive auth during Docker build” (no TTY, and rebuilds would re-run it anyway). The reliable pattern is:

1. persist each tool’s credential/config dir via a devcontainer volume mount, and
2. run an idempotent “first-run bootstrap” script only when you open an interactive shell (or via a VS Code task you run once).

That gives you “one-time interactive login” while still being part of the container setup story.

Claude Code’s own devcontainer docs also explicitly warn that a devcontainer can exfiltrate anything accessible inside it (including Claude credentials), especially if you run with `--dangerously-skip-permissions`, so keeping creds in a dedicated mounted dir is the right mental model. ([Claude][1])

Authentication + persistence strategy

Claude Code

* User settings live in `~/.claude/settings.json`. ([Claude][2])
* Persist `~/.claude` via a named volume mount so you don’t re-auth every rebuild.

Codex CLI

* Use `codex login --device-auth` for headless/devcontainer-friendly auth, or pipe an API key via `codex login --with-api-key` for non-browser flows. ([OpenAI Developers][3])
* Treat `~/.codex/auth.json` as a password; persist `~/.codex` via a named volume mount. ([OpenAI Developers][4])

Gemini CLI

* In headless/non-interactive environments, Gemini CLI expects env-based auth (e.g. `GEMINI_API_KEY`, or Vertex/ADC env vars). ([google-gemini.github.io][5])
* It supports `.env` loading (commonly `~/.gemini/.env` as a user-wide place). ([google-gemini.github.io][5])
* Persist `~/.gemini` via a named volume mount.

Oh-My-ClaudeCode without the interactive marketplace UI

Good news: Claude Code now has non-interactive plugin management commands. The official docs include `claude plugin install ...` etc. ([Claude][6])

So you can do “marketplace add + plugin install” from scripts instead of typing `/plugin ...` inside the TUI.

OMC’s own quick start is still “add marketplace, install plugin, run setup”. ([GitHub][7])
The only awkward bit is the final `/omc-setup` step, which is a slash command; you can either run it once in an interactive Claude Code session, or attempt to run it via Claude’s non-interactive prompt mode if you want it fully scripted (I’ll show both).

Concrete implementation: devcontainer mounts + first-run bootstrap

1. Add mounts for creds/config

In `.devcontainer/devcontainer.json`:

```json
{
  "mounts": [
    "source=claude-config,target=/home/vscode/.claude,type=volume",
    "source=codex-config,target=/home/vscode/.codex,type=volume",
    "source=gemini-config,target=/home/vscode/.gemini,type=volume",

    // optional: persist npm/pnpm caches if you rebuild often
    "source=npm-cache,target=/home/vscode/.npm,type=volume",
    "source=pnpm-store,target=/home/vscode/.pnpm-store,type=volume",

    // optional: if you use gcloud-based auth for Gemini Vertex / ADC
    // "source=${localEnv:HOME}/.config/gcloud,target=/home/vscode/.config/gcloud,type=bind,consistency=cached"
    // (works best on Linux/macOS hosts; Windows paths can be annoying)
    ""
  ],
  "containerEnv": {
    "PNPM_HOME": "/home/vscode/.local/share/pnpm",
    "PATH": "/home/vscode/.local/share/pnpm:/home/vscode/.local/bin:${containerEnv:PATH}"
  }
}
```

Notes:

* Replace `vscode` with whatever `remoteUser` is in the Anthropic devcontainer you’re basing off.
* The bind-mount for gcloud is optional; if you prefer API keys for Gemini, you don’t need it.

2. Add an idempotent “first interactive shell” script

Create `.devcontainer/bootstrap.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

MARKER="${HOME}/.devcontainer_bootstrapped"

if [[ -f "${MARKER}" ]]; then
  exit 0
fi

echo ""
echo "== Devcontainer bootstrap =="
echo "This runs once per persisted home-volume."
echo ""

# --- Claude Code auth ---
# Claude’s auth flow can vary by install channel; the point is: run it once,
# and because ~/.claude is a volume, it sticks.
if command -v claude >/dev/null 2>&1; then
  echo "-> Claude Code: starting (complete sign-in if prompted)..."
  echo "   Tip: you can Ctrl+C after sign-in if it drops you into a session."
  claude || true
else
  echo "-> Claude Code not found on PATH (skip)."
fi

# --- Codex auth ---
if command -v codex >/dev/null 2>&1; then
  if codex login status >/dev/null 2>&1; then
    echo "-> Codex already logged in."
  else
    echo "-> Codex login..."
    if [[ -n "${OPENAI_API_KEY:-}" ]]; then
      # API key mode (no browser)
      printenv OPENAI_API_KEY | codex login --with-api-key
    else
      # Headless-friendly device code flow
      codex login --device-auth
    fi
  fi
else
  echo "-> Codex CLI not found on PATH (skip)."
fi

# --- Gemini auth ---
if command -v gemini >/dev/null 2>&1; then
  mkdir -p "${HOME}/.gemini"
  if [[ -n "${GEMINI_API_KEY:-}" ]]; then
    # Persist via ~/.gemini/.env (Gemini CLI loads env vars from there)
    ENVFILE="${HOME}/.gemini/.env"
    if ! grep -q "^GEMINI_API_KEY=" "${ENVFILE}" 2>/dev/null; then
      echo "-> Writing GEMINI_API_KEY to ${ENVFILE}"
      umask 077
      {
        echo "GEMINI_API_KEY=\"${GEMINI_API_KEY}\""
      } >> "${ENVFILE}"
    else
      echo "-> GEMINI_API_KEY already present in ${ENVFILE}"
    fi
  else
    echo "-> Gemini: no GEMINI_API_KEY provided."
    echo "   For headless usage, Gemini expects env-based auth (API key or Vertex/ADC env vars)."
  fi
else
  echo "-> Gemini CLI not found on PATH (skip)."
fi

# --- Oh My Claude Code plugin install ---
# Use Claude's non-interactive plugin management CLI where possible.
if command -v claude >/dev/null 2>&1; then
  echo "-> Installing oh-my-claudecode plugin (if not present)..."

  # Depending on your Claude version, marketplace subcommands may exist.
  # If they don't, you can fall back to adding extraKnownMarketplaces in settings.json (see below).
  if claude plugin --help >/dev/null 2>&1; then
    # Try to add marketplace (best-effort; ignore failures if already added).
    claude plugin marketplace add Yeachan-Heo/oh-my-claudecode || true

    # Install plugin (name-only install often works once marketplace is known).
    claude plugin install oh-my-claudecode || true
  fi

  echo "-> OMC setup still needs to run once:"
  echo "   In Claude Code, run:  /oh-my-claudecode:omc-setup"
  echo "   (or /omc-setup depending on how it registers)"
fi

touch "${MARKER}"
echo "== Bootstrap complete =="
```

Why this shape:

* It’s idempotent via a marker file.
* It relies on mounted `~/.claude`, `~/.codex`, `~/.gemini` so “once” truly means once.

3. Make it run only on interactive shells

Add this to `.bashrc` (or `.zshrc` if you’re using zsh inside the container):

```bash
# Run devcontainer bootstrap only for interactive shells
if [[ $- == *i* ]] && [[ -f "${HOME}/.devcontainer_bootstrapped" || -f "${HOME}/.bashrc" ]]; then
  if [[ ! -f "${HOME}/.devcontainer_bootstrapped" ]] && [[ -x "/workspaces/.devcontainer/bootstrap.sh" ]]; then
    /workspaces/.devcontainer/bootstrap.sh
  fi
fi
```

How to wire it in automatically:

* Use `postCreateCommand` to append that snippet into the container user’s rc file, and `chmod +x .devcontainer/bootstrap.sh`.

Example `postCreateCommand`:

```json
{
  "postCreateCommand": "chmod +x .devcontainer/bootstrap.sh && grep -q devcontainer_bootstrap ~/.bashrc || cat >> ~/.bashrc <<'EOF'\n# devcontainer_bootstrap\nif [[ $- == *i* ]] && [[ ! -f \"${HOME}/.devcontainer_bootstrapped\" ]] && [[ -x \"/workspaces/.devcontainer/bootstrap.sh\" ]]; then\n  /workspaces/.devcontainer/bootstrap.sh\nfi\nEOF\n"
}
```

This avoids the classic trap: `postCreateCommand` itself is non-interactive, so it can’t safely run `codex login` / `claude` / etc.

If `claude plugin marketplace add ...` isn’t available on your version

The official docs clearly support non-interactive `claude plugin install ...` etc. ([Claude][6])
Marketplace CLI subcommands have existed in the wild for a while, but if your `claude` build doesn’t have them, you still have a clean fallback:

* Put marketplace configuration into `.claude/settings.json` under `extraKnownMarketplaces`, which Claude Code uses to prompt/install for the repo. ([Claude][8])

You can generate this file as part of the container setup (non-interactive) so the next time you open Claude Code it will offer to install it.

Example `.claude/settings.json`:

```json
{
  "extraKnownMarketplaces": {
    "omc": {
      "source": { "source": "github", "repo": "Yeachan-Heo/oh-my-claudecode" }
    }
  },
  "enabledPlugins": [
    "oh-my-claudecode@omc"
  ]
}
```

Then your “first run” becomes:

1. open `claude`
2. accept marketplace/plugin trust prompt
3. run `/oh-my-claudecode:omc-setup` ([Yeachan-Heo.github.io][9])

About fully scripting `/oh-my-claudecode:omc-setup`

If you’re determined to make even that “hands-off”, there are two practical options:

Option A (recommended): accept one manual step

* You still get everything automated except one slash command. This is usually the best tradeoff because it makes failures obvious and avoids brittle TUI automation.

Option B (try it): invoke setup via non-interactive Claude prompt mode

* Some folks run Claude Code in “print/non-interactive” mode (often `claude -p "...") and can execute slash commands that way, depending on the current CLI behavior. This isn’t as well-specified in the official docs as plugin CLI management, so I’d treat it as “best effort”: if it fails, fall back to Option A.

If you try Option B, add to bootstrap:

```bash
claude -p "/oh-my-claudecode:omc-setup" || true
```

If it doesn’t take, you’ll still have the echo telling you to run it manually.

Security footnote that matters in a persistent devcontainer

If you’re going to run Claude Code in a mode that skips permission prompts, Claude’s docs explicitly call out that a malicious repo can exfiltrate anything accessible in the container, including credentials. ([Claude][1])
So the “persist config dirs via volume” approach is good, but the bigger control is: don’t run these agents against untrusted repos in the same container that holds your tokens.

If you want, paste your current `.devcontainer/devcontainer.json` (or the Anthropic baseline you’re starting from) and I’ll splice these changes in cleanly and keep it aligned with how that container sets `remoteUser`, workspace mounts, and shells.

[1]: https://code.claude.com/docs/en/devcontainer "Development containers - Claude Code Docs"
[2]: https://code.claude.com/docs/en/settings?utm_source=chatgpt.com "Claude Code settings - Claude Code Docs"
[3]: https://developers.openai.com/codex/cli/reference/ "Command line options"
[4]: https://developers.openai.com/codex/auth/?utm_source=chatgpt.com "Authentication"
[5]: https://google-gemini.github.io/gemini-cli/docs/get-started/authentication.html "Gemini CLI Authentication Setup | gemini-cli"
[6]: https://code.claude.com/docs/en/plugins-reference "Plugins reference - Claude Code Docs"
[7]: https://github.com/Yeachan-Heo/oh-my-claudecode "GitHub - Yeachan-Heo/oh-my-claudecode: Teams-first Multi-agent orchestration for Claude Code"
[8]: https://code.claude.com/docs/en/discover-plugins "Discover and install prebuilt plugins through marketplaces - Claude Code Docs"
[9]: https://yeachan-heo.github.io/oh-my-claudecode-website/?utm_source=chatgpt.com "oh-my-claudecode - A weapon, not a tool"
