export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8
export EDITOR=nano
export VISUAL=nano
export SHELL=/bin/zsh

# Normalise TERM to a 256-colour entry the image actually ships before the theme
# loads. Two distinct failure modes, both of which break powerlevel10k:
#   1. A multiplexer hands us a bare 8-colour TERM ("screen"/"tmux"); p10k then
#      collapses its 256-colour palette to the nearest ANSI slot.
#   2. The host terminal hands us a TERM with no terminfo entry in the container
#      (e.g. Ghostty's "xterm-ghostty", or kitty/wezterm/foot's own entries);
#      p10k can't query colour support and renders the prompt monochrome.
# Upgrade the known multiplexer names first, then fall back to xterm-256color for
# anything the container's terminfo DB doesn't recognise.
case "${TERM}" in
  screen) export TERM=screen-256color ;;
  tmux)   export TERM=tmux-256color ;;
esac
if ! infocmp -- "${TERM}" >/dev/null 2>&1; then
  export TERM=xterm-256color
fi

[[ -f "${HOME}/.agent-shell-setup.sh" ]] && source "${HOME}/.agent-shell-setup.sh"
source "${HOME}/.powerlevel10k/powerlevel10k.zsh-theme"
[[ -f "${HOME}/.p10k.zsh" ]] && source "${HOME}/.p10k.zsh"

autoload -Uz compinit
compinit
