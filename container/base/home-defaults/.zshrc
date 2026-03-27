export LANG=en_US.UTF-8
export LC_ALL=en_US.UTF-8
export EDITOR=nano
export VISUAL=nano
export SHELL=/bin/zsh

[[ -f "${HOME}/.agent-shell-setup.sh" ]] && source "${HOME}/.agent-shell-setup.sh"
source "${HOME}/.powerlevel10k/powerlevel10k.zsh-theme"
[[ -f "${HOME}/.p10k.zsh" ]] && source "${HOME}/.p10k.zsh"

autoload -Uz compinit
compinit
