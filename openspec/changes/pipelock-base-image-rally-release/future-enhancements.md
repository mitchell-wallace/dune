# Future Enhancements

Deferred items that may be worth revisiting after the initial implementation is stable.

## Pipelock Health Check

Add a Docker health check to the Pipelock service in compose.yaml and make the agent service `depends_on` with `condition: service_healthy`. This would prevent the agent container from starting before Pipelock is ready and surface Pipelock failures via Docker's health reporting. Deferred because Pipelock may be stable enough that this isn't needed — observe real usage first.

## Per-Repo Pipelock Config Overrides

Allow a `pipelock.yaml` in the repo root to extend or override the global `~/.config/dune/pipelock.yaml`. Useful for repos that need additional allowlisted domains or tighter policies. Deferred because global config is sufficient for single-user usage.

## SSH Passthrough

Git SSH (`git@github.com:...`) doesn't route through HTTP proxies. Adding SSH agent forwarding or a SOCKS proxy for SSH traffic would enable SSH-based git remotes. Deferred because HTTPS is the default git transport and handles most workflows.
