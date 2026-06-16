typeset -g POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true
typeset -g POWERLEVEL9K_LEFT_PROMPT_ELEMENTS=(dir vcs prompt_char)
typeset -g POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS=(status command_execution_time time)
typeset -g POWERLEVEL9K_PROMPT_ADD_NEWLINE=true
typeset -g POWERLEVEL9K_MODE=nerdfont-complete

# Workspace/dir segment: white on medium blue (visible on light and dark terminals).
typeset -g POWERLEVEL9K_DIR_FOREGROUND=255
typeset -g POWERLEVEL9K_DIR_BACKGROUND=31

# Git/vcs segment: light text on dark backgrounds for high contrast.
typeset -g POWERLEVEL9K_VCS_CLEAN_FOREGROUND=255
typeset -g POWERLEVEL9K_VCS_CLEAN_BACKGROUND=22
typeset -g POWERLEVEL9K_VCS_MODIFIED_FOREGROUND=255
typeset -g POWERLEVEL9K_VCS_MODIFIED_BACKGROUND=124

# Time segment (right): light text on dark slate so it never blends into the
# terminal background.
typeset -g POWERLEVEL9K_TIME_FOREGROUND=250
typeset -g POWERLEVEL9K_TIME_BACKGROUND=235
