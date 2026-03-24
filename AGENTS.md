When addressing container build issues, you should try building an ephemeral container to test things in. You have permission to do this.

When working through a plan, you should break your work into commits to checkpoint progress. Make sure that each commit leaves a state which builds successfully, unless explicitly requested to make a (wip) commit. After completing a plan, all work should be committed.

Ad-hoc work that forms a sizeable change should come with an offer to commit, pending confirmation of build status.

## Architecture: host vs container

The container's `/workspace` contains the **user's project**, not the claudebox/dune/rally source code. The `dune` CLI running on the host is the only channel for transferring host-side artifacts (like rebuilt binaries) into the container. Rally inside the container cannot self-update or access its own source. Use `dune rally build` to push an updated rally binary into a running container.
