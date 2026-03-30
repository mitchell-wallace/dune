The planned integration test suite (12 tests) covers the critical paths but has significant gaps:

Missing tests I'd add:

Proxy bypass verification — Attempt raw TCP connection from agent container to an external IP. Must fail. This is the security-critical test.
Git HTTPS clone through proxy — git clone https://github.com/... from inside agent container. This is the most common agent operation.
Pipelock crash recovery — Kill Pipelock container, verify restart policy brings it back, verify agent requests succeed after recovery.
DLP false positive handling — Agent writes code containing a string matching a DLP pattern (e.g., a test fixture with a fake AWS key). Verify it's warned but not blocked.
Concurrent profile isolation — Start two profiles for the same workspace simultaneously. Verify they get separate containers, separate volumes, no port conflicts.
Volume migration from old naming — Start with an agent-persist-0 volume containing credentials, run new dune, verify credentials are accessible.
Dockerfile.dune error handling — Provide a broken Dockerfile.dune, verify dune gives a clear error and doesn't leave partial state.
Cold pull timing — Time dune up on a machine with no cached images. Validate the 2-3GB image pull is the bottleneck, not something else.