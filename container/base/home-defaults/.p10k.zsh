typeset -g POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true
typeset -g POWERLEVEL9K_LEFT_PROMPT_ELEMENTS=(dir vcs prompt_char)
typeset -g POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS=(status command_execution_time time)
typeset -g POWERLEVEL9K_PROMPT_ADD_NEWLINE=true
typeset -g POWERLEVEL9K_MODE=nerdfont-complete

# Palette principle: every segment carries an explicit mid-tone background with
# light text. Mid-tone backgrounds keep their contrast against BOTH light and
# dark terminals, so the prompt reads the same regardless of the user's theme.
# Colours are 256-palette indices (not 24-bit), so they render identically with
# or without truecolor and pass through a 256-colour tmux unchanged.

# Directory: white on medium blue.
typeset -g POWERLEVEL9K_DIR_FOREGROUND=255
typeset -g POWERLEVEL9K_DIR_BACKGROUND=25

# Git/VCS: green when clean, amber when there is anything to commit. Amber (not
# red) keeps red reserved for genuine errors (prompt char / status) below.
typeset -g POWERLEVEL9K_VCS_CLEAN_FOREGROUND=255
typeset -g POWERLEVEL9K_VCS_CLEAN_BACKGROUND=28
typeset -g POWERLEVEL9K_VCS_MODIFIED_FOREGROUND=255
typeset -g POWERLEVEL9K_VCS_MODIFIED_BACKGROUND=166
typeset -g POWERLEVEL9K_VCS_UNTRACKED_FOREGROUND=255
typeset -g POWERLEVEL9K_VCS_UNTRACKED_BACKGROUND=166

# Prompt char: green on success, red on error. No background, so the foregrounds
# are mid-tones chosen to read on both light and dark terminals.
typeset -g POWERLEVEL9K_PROMPT_CHAR_OK_{VIINS,VICMD,VIVIS,VIOWR}_FOREGROUND=70
typeset -g POWERLEVEL9K_PROMPT_CHAR_ERROR_{VIINS,VICMD,VIVIS,VIOWR}_FOREGROUND=160

# Exit status (right): green check / red cross, no background.
typeset -g POWERLEVEL9K_STATUS_OK_FOREGROUND=70
typeset -g POWERLEVEL9K_STATUS_ERROR_FOREGROUND=160

# Command execution time (right): white on amber-brown.
typeset -g POWERLEVEL9K_COMMAND_EXECUTION_TIME_FOREGROUND=255
typeset -g POWERLEVEL9K_COMMAND_EXECUTION_TIME_BACKGROUND=94

# Clock (right): white on slate. The explicit mid-tone background is the fix for
# the invisible clock -- the previous near-black background blended into dark
# terminals so the time was unreadable.
typeset -g POWERLEVEL9K_TIME_FOREGROUND=255
typeset -g POWERLEVEL9K_TIME_BACKGROUND=60
