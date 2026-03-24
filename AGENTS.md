When addressing container build issues, you should try building an ephemeral container to test things in. You have permission to do this. Note that `dune.sh` is the canonical entry point and needs to work.
If a package or installer is part of the core container build, do not paper over failures by skipping it. Fix the install path so the tool actually installs successfully in the image.

When working through a plan, you should break your work into commits to checkpoint progress. Make sure that each commit leaves a state which builds successfully (docker build or go build as appropriate). After completing a plan, all work should be committed.

Ad-hoc work that forms a sizeable change should come with an offer to commit, pending confirmation of build status.

## Architecture: host vs container

The container's `/workspace` contains the **user's project**, not the claudebox/dune/rally source code. The `dune` CLI running on the host is the only channel for transferring host-side artifacts (like rebuilt binaries) into the container. Rally inside the container cannot self-update or access its own source. `dune rally build` rebuilds the host system rally binary and updates the current repo container; `dune rally update` re-pushes the latest host system rally binary into the current repo container.
