export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8
export EDITOR=nano
export VISUAL=nano
export SHELL=/bin/zsh

# Terminal multiplexers commonly hand us an 8-colour TERM ("screen"/"tmux").
# Powerlevel10k then collapses its 256-colour palette to the nearest ANSI slot,
# which mangles the prompt. Upgrade to the 256-colour variant (the image ships
# both terminfo entries) before the theme loads so the palette renders as-is.
case "${TERM}" in
  screen) export TERM=screen-256color ;;
  tmux)   export TERM=tmux-256color ;;
esac

[[ -f "${HOME}/.agent-shell-setup.sh" ]] && source "${HOME}/.agent-shell-setup.sh"
source "${HOME}/.powerlevel10k/powerlevel10k.zsh-theme"
[[ -f "${HOME}/.p10k.zsh" ]] && source "${HOME}/.p10k.zsh"

autoload -Uz compinit
compinit
