set shell := ["bash", "-euo", "pipefail", "-c"]

golangci_lint_module := "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8"
shellcheck_image := "koalaman/shellcheck:stable"
hadolint_image := "hadolint/hadolint:latest"
shellcheck_files := `find . -type f \( -name '*.sh' -o -path './container/base/s6-overlay/*/run' -o -path './container/base/s6-overlay/*/up' \) | sort | tr '\n' ' '`

default:
  @just --list

golangci:
  @if command -v golangci-lint >/dev/null 2>&1; then \
    golangci-lint run; \
  else \
    go run {{golangci_lint_module}} run; \
  fi

shellcheck:
  @if command -v shellcheck >/dev/null 2>&1; then \
    shellcheck {{shellcheck_files}}; \
  else \
    docker run --rm -v "$PWD:/work" -w /work {{shellcheck_image}} {{shellcheck_files}}; \
  fi

hadolint:
  @if command -v hadolint >/dev/null 2>&1; then \
    hadolint --failure-threshold error Dockerfile; \
  else \
    docker run --rm -i {{hadolint_image}} hadolint --failure-threshold error - < Dockerfile; \
  fi

test: golangci shellcheck hadolint
  go test ./...
