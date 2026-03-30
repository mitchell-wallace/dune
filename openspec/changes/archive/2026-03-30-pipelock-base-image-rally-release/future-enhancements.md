# Future Enhancements

Deferred items that may be worth revisiting after the initial implementation is stable.

## Pipelock Health Check

Add a Docker health check to the Pipelock service in compose.yaml and make the agent service `depends_on` with `condition: service_healthy`. This would prevent the agent container from starting before Pipelock is ready and surface Pipelock failures via Docker's health reporting. Deferred because Pipelock may be stable enough that this isn't needed — observe real usage first.

## Per-Repo Pipelock Config Overrides

Allow a `pipelock.yaml` in the repo root to extend or override the global `~/.config/dune/pipelock.yaml`. Useful for repos that need additional allowlisted domains or tighter policies. Deferred because global config is sufficient for single-user usage.

## SSH Passthrough

Git SSH (`git@github.com:...`) doesn't route through HTTP proxies. Adding SSH agent forwarding or a SOCKS proxy for SSH traffic would enable SSH-based git remotes. Deferred because HTTPS is the default git transport and handles most workflows.

## Compose Extensions for Additional Services

The current compose topology is a fixed two-container setup (agent + pipelock) generated from an embedded Go template. Users who need additional services (Elasticsearch, RabbitMQ, a second database, etc.) have no extension point beyond `Dockerfile.dune` (which only customises the agent image, not the service topology). A future enhancement could support an optional `docker-compose.dune.yaml` in the workspace root that gets merged with the generated compose file via Docker Compose's native `-f` multi-file support. This would let users add per-repo services without modifying the dune CLI. Deferred because the current batteries-included image (postgres, redis, mailpit) covers most agent workflows, and `Dockerfile.dune` handles per-repo tool installation within the agent container.
