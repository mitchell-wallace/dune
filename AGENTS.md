When addressing container build issues, you should try building an ephemeral container to test things in. You have permission to do this. The compiled `dune` Go binary is the canonical entry point.
If a package or installer is part of the core container build, do not paper over failures by skipping it. Fix the install path so the tool actually installs successfully in the image.

When working through a plan, you should break your work into commits to checkpoint progress. Make sure that each commit leaves a state which builds successfully (docker build or go build as appropriate). After completing a plan, all work should be committed.

Ad-hoc work that forms a sizeable change should come with an offer to commit, pending confirmation of build status.

## Architecture: host vs container

The container's `/workspace` contains the **user's project**, not the dune source code. The host-side `dune` CLI is responsible for generating the compose file, creating the profile-specific persist volume, and starting the `agent` and `pipelock` containers.

Rally is an independently released tool that is installed into the base image from GitHub Releases and can self-update inside the container. Repo-specific Rally configuration lives in `/workspace/rally.toml`.
