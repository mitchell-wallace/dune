# shellcheck shell=sh
# /etc/profile.d/dune-setup-persist.sh - Dune setup-persist boot hook (bash/sh)
#
# Sourced automatically by bash/sh login shells. Sources the sentinel-guarded
# hook that runs setup-persist.sh once per sandbox instance.

# shellcheck source=container/sbx/etc/dune/setup-persist-hook.sh
if [ -f /etc/dune/setup-persist-hook.sh ]; then
  . /etc/dune/setup-persist-hook.sh
fi
