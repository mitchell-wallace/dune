# Container Runtime

Container-native behavior remains shell-based under `container/runtime/`.

Responsibilities:

- entrypoint and post-start setup
- agent persistence symlinks and default seeding
- privileged root command dispatch
- firewall initialization
- addon execution and local service helpers

The host CLI treats these scripts as stable runtime contracts rather than reimplementing them in Go.
