#!/usr/bin/env python3
"""Prepare a reproducible panel kernel build for the current core git commit.

The core release workflow checks out SawaMEN/3x-ui for overlays, then rewrites
only this disposable checkout's manifest and expected process identity.
"""
import difflib
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def gitlink(name):
    fields = command("git", "ls-tree", "HEAD", "--", name).split()
    if len(fields) != 4 or fields[0:2] != ["160000", "commit"] or fields[3] != name:
        raise ValueError("Missing gitlink: " + name)
    if not re.fullmatch(r"[0-9a-f]{40}", fields[2]):
        raise ValueError("Invalid gitlink: " + name)
    return fields[2]


def prepare_runtime_dependencies(panel, module):
    """Rebase the panel's additive dependency overlay onto this exact core."""
    patch = panel / "deploy/hiddify/runtime-dependencies.patch"
    source = patch.read_text()
    if not source.startswith("--- a/go.mod\n+++ b/go.mod\n"):
        raise ValueError("Unexpected runtime dependency overlay target")
    additions = []
    for line in source.splitlines():
        if line.startswith("-") and not line.startswith("--- "):
            raise ValueError("Runtime dependency overlay must only add requirements")
        if not line.startswith("+") or line.startswith("+++ "):
            continue
        match = re.fullmatch(r"\+\s*([A-Za-z0-9._~/-]+) (v[^\s]+)(?: // indirect)?", line)
        if not match:
            raise ValueError("Unexpected runtime dependency overlay change")
        name, version = match.groups()
        if not re.search(r"(?m)^\s*" + re.escape(name) + r"\s+", module):
            additions.append(f"\t{name} {version} // indirect\n")
    updated = module
    if additions:
        if "require (\n" not in module:
            raise ValueError("Missing core requirement block")
        updated = module.replace("require (\n", "require (\n" + "".join(additions), 1)
    patch.write_text("".join(difflib.unified_diff(
        module.splitlines(keepends=True), updated.splitlines(keepends=True),
        fromfile="a/go.mod", tofile="b/go.mod",
    )))
    # An already satisfied overlay is empty and needs no git apply invocation.
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    command = 'git -C "$task_source/core" apply "$task_root/deploy/hiddify/runtime-dependencies.patch"'
    if script.count(command) != 1:
        raise ValueError("Cannot locate runtime dependency overlay application")
    script = script.replace(command, '[ ! -s "$task_root/deploy/hiddify/runtime-dependencies.patch" ] || ' + command)
    build.write_text(script)


def refresh_go_module_source_check(lock, module):
    """Keep the panel's exact Go toolchain evidence aligned with this checked-out core."""
    match = re.search(r"(?m)^go [0-9]+\.[0-9]+(?:\.[0-9]+)?$", module)
    if match is None:
        raise ValueError("Missing Go toolchain declaration in core go.mod")
    checks = [check for check in lock["source_checks"] if check.get("path") == "go.mod"]
    if len(checks) != 1 or not re.fullmatch(r"go [0-9]+\.[0-9]+(?:\.[0-9]+)?", checks[0].get("contains", "")):
        raise ValueError("Unexpected panel Go module source check")
    checks[0]["contains"] = match.group(0)
    # Release builder runs with GOTOOLCHAIN pinned; an older Go patch version
    # cannot compile a module requiring a newer language/toolchain version.
    panel_build = lock.get("panel_build")
    if not isinstance(panel_build, dict) or not re.fullmatch(
        r"go[0-9]+\.[0-9]+(?:\.[0-9]+)?", str(panel_build.get("toolchain", ""))
    ):
        raise ValueError("Unexpected panel Go toolchain setting")
    panel_build["toolchain"] = "go" + match.group(0)[3:]


def refresh_panel_audit_toolchain_check(panel, lock):
    """Pin the panel's disposable runtime audit to the checked-out core Go version."""
    toolchain = lock["panel_build"]["toolchain"]
    if not re.fullmatch(r"go[0-9]+\.[0-9]+(?:\.[0-9]+)?", toolchain):
        raise ValueError("Invalid runtime audit toolchain")
    audit_file = panel / "tools/audit-hiddify-migration.py"
    audit = audit_file.read_text()
    check = r'require\(build\["toolchain"\] == "go[0-9]+\.[0-9]+(?:\.[0-9]+)?", "Runtime must use the pinned security-patched toolchain"\)'
    updated, count = re.subn(
        check,
        'require(build["toolchain"] == "' + toolchain + '", "Runtime must use the pinned security-patched toolchain")',
        audit,
    )
    if count != 1:
        raise ValueError("Cannot update panel runtime audit toolchain check")
    audit_file.write_text(updated)


def patch_profile_suffix(version):
    """Preserve known numbered panel/mask and optional cipher profiles."""
    if not isinstance(version, str):
        raise ValueError("Invalid patch profile")
    match = re.search(r"(-(?:panel|mask)[0-9]+(?:-ciphers[0-9]+)?)$", version)
    if not match:
        raise ValueError("Cannot determine patch profile (expected -panelN or -maskN)")
    return match.group(1)


def prepare(panel):
    core_sha = os.environ["KERNEL_COMMIT"]
    channel = os.environ["KERNEL_CHANNEL"]
    if not re.fullmatch(r"[0-9a-f]{40}", core_sha):
        raise ValueError("Invalid kernel revision")
    if channel not in {"dev", "stable"}:
        raise ValueError("Invalid release channel")
    if command("git", "rev-parse", "HEAD") != core_sha:
        raise ValueError("Checked-out kernel is not the triggering commit")
    pinned_sing_sha, ray_sha = gitlink("hiddify-sing-box"), gitlink("ray2sing")
    # CI resolves the current fork revision once per workflow, so both
    # architectures use identical sources. Local builds retain the gitlink.
    sing_sha = os.environ.get("SINGBOX_COMMIT") or pinned_sing_sha
    if not re.fullmatch(r"[0-9a-f]{40}", sing_sha):
        raise ValueError("Invalid Sing-box revision")

    prepare_runtime_dependencies(panel, Path("go.mod").read_text())

    lock_file = panel / "docs/hiddify-core-target.json"
    lock = json.loads(lock_file.read_text())
    refresh_go_module_source_check(lock, Path("go.mod").read_text())
    refresh_panel_audit_toolchain_check(panel, lock)
    target = lock["target"]
    if target["repository"] != "https://github.com/SawaMEN/hiddify-core":
        raise ValueError("Unexpected kernel repository")
    # Panel's current lock can use -maskN in addition to older -panelN.
    # Keep the exact suffix so all release assets agree on their identity.
    suffix = patch_profile_suffix(target["version"])
    prefix = "sawamen-dev" if channel == "dev" else "sawamen"
    version = f"{prefix}-{core_sha[:12]}-{sing_sha[:12]}{suffix}"
    target["commit"] = core_sha
    target["version"] = version
    target["submodules"]["hiddify-sing-box"] = sing_sha
    target["submodules"]["ray2sing"] = ray_sha
    lock["panel_build"]["sing_box_version"] = "sawamen-" + sing_sha[:12]
    lock_file.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n")

    profile_file = panel / "internal/hiddify/translate.go"
    profile = profile_file.read_text()
    for name, value in (("TargetVersion", version), ("TargetRevision", core_sha)):
        expression = r'(?m)^([ \t]*' + name + r'[ \t]*=[ \t]*)"[^"\n]+"'
        profile, count = re.subn(expression, lambda m: m.group(1) + '"' + value + '"', profile)
        if count != 1:
            raise ValueError("Cannot set " + name)
    profile_file.write_text(profile)
    print(f"Kernel channel={channel} version={version} commit={core_sha}; Sing-box={sing_sha}; ray2sing={ray_sha}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: prepare-panel-kernel.py PANEL_CHECKOUT")
    prepare(Path(sys.argv[1]))
