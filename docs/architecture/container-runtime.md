# Container Runtime

The runtime is now centered on the base image, s6 services, and a small set of setup
scripts under `container/base/`.

Responsibilities:

- seed default home-directory files into `/persist/agent` on first boot
- create the persisted symlinks used by agent CLIs and shell config
- install Rally from GitHub Releases during image build
- configure in-container agent tools during the image build
- run PostgreSQL, Redis, and Mailpit under s6 supervision

The host-side `dune` CLI generates compose config and starts containers, while the
container image owns its own boot-time setup and supervised services.
