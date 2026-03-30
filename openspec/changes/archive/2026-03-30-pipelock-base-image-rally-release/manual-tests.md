# Manual Verification Steps

These tests require human judgment, real authentication, or visual inspection. Run after completing all automated tests.

## First-Run Experience

- [ ] Run `dune` in a fresh repo with no prior setup — verify the output is clear and the container starts without errors
- [ ] Verify the zsh prompt renders Powerlevel10k correctly (visual inspection)
- [ ] Verify `dune logs pipelock` output is readable and useful for debugging

## OAuth & Authentication

- [ ] Authenticate with Claude Code via `claude auth` inside the container
- [ ] Run `dune down && dune up` — verify the OAuth token persists (no re-authentication needed)
- [ ] Run `dune rebuild` — verify the OAuth token persists through image rebuild
- [ ] Authenticate with `gh auth login` — verify GitHub CLI credentials persist across restarts

## Profile UX

- [ ] Run `dune profile set work` then `dune` — verify the profile is used
- [ ] Run `dune profile list` — verify output is clear and shows the current directory's profile
- [ ] Switch profiles and verify credential isolation: work profile should not see personal profile's tokens

## Playwright

- [ ] Verify Playwright can take a screenshot of a public website through the Pipelock proxy
