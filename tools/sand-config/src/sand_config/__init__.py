from __future__ import annotations

import argparse
import csv
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

import questionary
from questionary import Choice
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from tomlkit import array, document, dumps, parse
from tomlkit.toml_document import TOMLDocument

PROFILE_RE = re.compile(r"^[0-9a-zA-Z]$")
PROFILE_VOLUME_RE = re.compile(r"^agent-persist-([0-9a-zA-Z])$")
MODE_HELP: dict[str, str] = {
    "std": "firewall enabled, curated addons available",
    "lax": "firewall enabled, passwordless sudo",
    "yolo": "firewall disabled, passwordless sudo",
    "strict": "firewall enabled, addons disabled",
}
VERSION_KEYS = [
    "python_version",
    "uv_version",
    "go_version",
    "rust_version",
    "dotnet_version",
    "java_version",
    "maven_version",
    "gradle_version",
    "bun_version",
    "deno_version",
]


@dataclass(frozen=True)
class Addon:
    name: str
    description: str
    enabled_modes: tuple[str, ...]


def normalize_profile(raw: str) -> str | None:
    value = raw.strip().lower()
    if PROFILE_RE.fullmatch(value):
        return value
    return None


def canonicalize_mode(raw: str | None) -> str:
    if raw is None:
        return "std"
    value = raw.strip().lower()
    if value in {"std", "standard"}:
        return "std"
    if value in MODE_HELP:
        return value
    return "std"


def profile_sort_key(profile: str) -> tuple[int, str]:
    return (0 if profile.isdigit() else 1, profile)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="sand-config",
        description="Interactive wizard to create/update sand.toml.",
    )
    parser.add_argument(
        "--directory",
        default=".",
        help="Workspace directory to inspect (default: current directory).",
    )
    parser.add_argument(
        "--manifest",
        required=True,
        help="Path to addons manifest.tsv file.",
    )
    parser.add_argument(
        "--repo-root",
        default="",
        help="Internal/testing override for repository root.",
    )
    return parser.parse_args(argv)


def run_checked(cmd: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, capture_output=True, text=True, check=False)


def resolve_repo_root(directory: Path, override: str) -> Path:
    if override:
        return Path(override).expanduser().resolve()

    result = run_checked(["git", "-C", str(directory), "rev-parse", "--show-toplevel"])
    if result.returncode == 0:
        out = result.stdout.strip()
        if out:
            return Path(out).resolve()
    return directory.resolve()


def parse_addons_manifest(path: Path) -> list[Addon]:
    addons: list[Addon] = []
    with path.open("r", encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        for row in reader:
            name = (row.get("name") or "").strip()
            description = (row.get("description") or "").strip()
            enabled_modes = tuple(
                mode.strip()
                for mode in (row.get("enabled_modes") or "").split(",")
                if mode.strip()
            )
            if not name:
                continue
            addons.append(
                Addon(
                    name=name,
                    description=description,
                    enabled_modes=enabled_modes,
                )
            )
    return addons


def discover_profiles() -> tuple[list[str], str | None]:
    if shutil.which("docker") is None:
        return [], "Docker was not found in PATH; profile discovery unavailable."

    result = run_checked(["docker", "volume", "ls", "--format", "{{.Name}}"])
    if result.returncode != 0:
        stderr = result.stderr.strip() or "unknown error"
        return [], f"Failed to list Docker volumes: {stderr}"

    profiles: set[str] = set()
    for line in result.stdout.splitlines():
        line = line.strip()
        match = PROFILE_VOLUME_RE.fullmatch(line)
        if not match:
            continue
        profile = normalize_profile(match.group(1))
        if profile is not None:
            profiles.add(profile)

    ordered = sorted(profiles, key=profile_sort_key)
    return ordered, None


def load_existing_toml(path: Path) -> TOMLDocument:
    if not path.exists():
        return document()

    content = path.read_text(encoding="utf-8")
    return parse(content)


def get_existing_scalar(doc: TOMLDocument, key: str) -> str:
    value = doc.get(key)
    if isinstance(value, str):
        return value
    return ""


def get_existing_addons(doc: TOMLDocument) -> set[str]:
    raw = doc.get("addons")
    if not isinstance(raw, list):
        return set()
    return {value for value in raw if isinstance(value, str)}


def prompt_profile(console: Console, discovered: list[str], default_profile: str) -> str:
    choices: list[Choice | str] = []
    for profile in discovered:
        choices.append(Choice(title=f"profile {profile}", value=profile))
    choices.append(Choice(title="Custom profile", value="__custom__"))

    if default_profile in discovered:
        default_value = default_profile
    else:
        default_value = discovered[0] if discovered else "__custom__"

    selected = questionary.select(
        "Select profile (persisted auth/agent/git state):",
        choices=choices,
        default=default_value,
    ).ask()
    if selected is None:
        raise KeyboardInterrupt

    if selected != "__custom__":
        return selected

    while True:
        entered = questionary.text(
            "Enter profile identifier (single char: 0-9 or a-z):",
            default=default_profile or "0",
        ).ask()
        if entered is None:
            raise KeyboardInterrupt
        normalized = normalize_profile(entered)
        if normalized is not None:
            return normalized
        console.print("[red]Invalid profile. Use one character: 0-9 or a-z.[/red]")


def prompt_mode(default_mode: str) -> str:
    choices: list[Choice] = []
    for mode, description in MODE_HELP.items():
        choices.append(Choice(title=f"{mode:6} {description}", value=mode))

    selected = questionary.select(
        "Select security mode:",
        choices=choices,
        default=default_mode,
    ).ask()
    if selected is None:
        raise KeyboardInterrupt
    return selected


def prompt_addons(
    mode: str,
    addons: list[Addon],
    existing_addons: set[str],
    console: Console,
) -> list[str]:
    if mode == "strict":
        table = Table(title="Addons (read-only in strict mode)")
        table.add_column("Addon")
        table.add_column("Configured")
        table.add_column("Available In")
        table.add_column("Description")
        for addon in addons:
            configured = "yes" if addon.name in existing_addons else "no"
            table.add_row(
                addon.name,
                configured,
                ",".join(addon.enabled_modes),
                addon.description,
            )
        console.print(table)
        console.print(
            "[yellow]strict mode disables addons. Wizard will write addons = [][/yellow]"
        )
        return []

    choices: list[Choice] = []
    for addon in addons:
        enabled_for_mode = mode in addon.enabled_modes
        note = "available" if enabled_for_mode else f"not available in {mode}"
        choices.append(
            Choice(
                title=f"{addon.name:16} {addon.description} [{note}]",
                value=addon.name,
                checked=enabled_for_mode and addon.name in existing_addons,
                disabled=None if enabled_for_mode else f"disabled in {mode}",
            )
        )

    selected = questionary.checkbox(
        "Select addons to configure in sand.toml:",
        choices=choices,
    ).ask()
    if selected is None:
        raise KeyboardInterrupt

    selected_set = set(selected)
    return [addon.name for addon in addons if addon.name in selected_set]


def prompt_versions(
    existing_values: dict[str, str],
    console: Console,
) -> dict[str, str | None] | None:
    configure = questionary.confirm(
        "Configure advanced runtime version pins?",
        default=False,
    ).ask()
    if configure is None:
        raise KeyboardInterrupt
    if not configure:
        return None

    console.print(
        "[dim]Leave a field blank to remove that key from sand.toml.[/dim]"
    )

    updates: dict[str, str | None] = {}
    for key in VERSION_KEYS:
        current = existing_values.get(key, "")
        value = questionary.text(
            f"{key}:",
            default=current,
        ).ask()
        if value is None:
            raise KeyboardInterrupt
        stripped = value.strip()
        updates[key] = stripped if stripped else None
    return updates


def update_doc(
    doc: TOMLDocument,
    profile: str,
    mode: str,
    addons: list[str],
    version_updates: dict[str, str | None] | None,
) -> None:
    doc["profile"] = profile
    doc["mode"] = mode
    addon_array = array()
    addon_array.multiline(False)
    for addon in addons:
        addon_array.append(addon)
    doc["addons"] = addon_array

    if version_updates is None:
        return

    for key in VERSION_KEYS:
        value = version_updates.get(key)
        if value:
            doc[key] = value
        else:
            doc.pop(key, None)


def print_summary(
    console: Console,
    repo_root: Path,
    target_path: Path,
    profile: str,
    mode: str,
    addons: list[str],
    version_updates: dict[str, str | None] | None,
    existing_versions: dict[str, str],
) -> None:
    table = Table(title="sand.toml review")
    table.add_column("Key")
    table.add_column("Value")
    table.add_row("repo_root", str(repo_root))
    table.add_row("target", str(target_path))
    table.add_row("profile", profile)
    table.add_row("mode", mode)
    table.add_row("addons", ", ".join(addons) if addons else "(none)")

    if version_updates is None:
        table.add_row("version pins", "unchanged")
    else:
        for key in VERSION_KEYS:
            value = version_updates.get(key)
            if value is None:
                table.add_row(key, "(removed)")
            else:
                table.add_row(key, value)

    if version_updates is None and existing_versions:
        table.add_row(
            "existing advanced keys",
            ", ".join(sorted(existing_versions.keys())),
        )

    console.print(table)


def validate_doc_shape(doc: TOMLDocument, console: Console) -> bool:
    for key in ("profile", "mode", *VERSION_KEYS):
        value = doc.get(key)
        if value is None:
            continue
        if not isinstance(value, str):
            console.print(
                f"[red]Existing sand.toml key '{key}' is not a string; keeping as-is until overwritten.[/red]"
            )
    addons = doc.get("addons")
    if addons is not None and not isinstance(addons, list):
        console.print("[yellow]Existing 'addons' key is not a list; wizard will replace it.[/yellow]")
    return True


def main() -> None:
    args = parse_args(sys.argv[1:])
    console = Console()

    workspace_dir = Path(args.directory).expanduser()
    if not workspace_dir.is_dir():
        console.print(f"[red]Workspace directory does not exist: {workspace_dir}[/red]")
        sys.exit(1)

    manifest_path = Path(args.manifest).expanduser().resolve()
    if not manifest_path.is_file():
        console.print(f"[red]Manifest file not found: {manifest_path}[/red]")
        sys.exit(1)

    addons = parse_addons_manifest(manifest_path)
    repo_root = resolve_repo_root(workspace_dir.resolve(), args.repo_root)
    target_path = repo_root / "sand.toml"

    try:
        doc = load_existing_toml(target_path)
    except Exception as exc:
        console.print(f"[red]Failed to parse existing sand.toml: {exc}[/red]")
        sys.exit(1)

    validate_doc_shape(doc, console)

    existing_profile = normalize_profile(get_existing_scalar(doc, "profile") or "") or "0"
    existing_mode = canonicalize_mode(get_existing_scalar(doc, "mode"))
    existing_addons = get_existing_addons(doc)
    existing_versions = {
        key: get_existing_scalar(doc, key)
        for key in VERSION_KEYS
        if get_existing_scalar(doc, key)
    }

    discovered_profiles, profile_warning = discover_profiles()

    panel_lines = [
        "This wizard creates/updates sand.toml at repo root.",
        "Profiles map to persisted Docker volumes (agent/auth/git settings persist by profile).",
        f"Repo root: {repo_root}",
        f"Target: {target_path}",
    ]
    console.print(
        Panel.fit(
            "\n".join(panel_lines),
            title="sand config",
            border_style="cyan",
        )
    )

    if profile_warning:
        console.print(f"[yellow]{profile_warning}[/yellow]")

    if discovered_profiles:
        console.print(
            f"[green]Discovered persisted profiles:[/green] {', '.join(discovered_profiles)}"
        )
    else:
        console.print("[dim]No persisted profiles discovered.[/dim]")

    try:
        profile = prompt_profile(console, discovered_profiles, existing_profile)
        mode = prompt_mode(existing_mode)
        selected_addons = prompt_addons(mode, addons, existing_addons, console)
        version_updates = prompt_versions(existing_versions, console)
    except KeyboardInterrupt:
        console.print("\n[yellow]Configuration cancelled.[/yellow]")
        sys.exit(1)

    print_summary(
        console=console,
        repo_root=repo_root,
        target_path=target_path,
        profile=profile,
        mode=mode,
        addons=selected_addons,
        version_updates=version_updates,
        existing_versions=existing_versions,
    )

    confirm = questionary.confirm("Write changes to sand.toml?", default=True).ask()
    if confirm is None:
        console.print("[yellow]Configuration cancelled.[/yellow]")
        sys.exit(1)
    if not confirm:
        console.print("[yellow]No changes written.[/yellow]")
        return

    update_doc(
        doc=doc,
        profile=profile,
        mode=mode,
        addons=selected_addons,
        version_updates=version_updates,
    )

    rendered = dumps(doc)
    if not rendered.endswith("\n"):
        rendered += "\n"
    target_path.write_text(rendered, encoding="utf-8")

    console.print(f"[green]Wrote[/green] {target_path}")
    console.print(f"Next: [bold]sand {profile} {mode}[/bold] or just [bold]sand[/bold]")
