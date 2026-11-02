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


def configure_panel_protection_rebase(panel):
    """Rebase protection patch in the disposable panel checkout, never silently skip it."""
    source = Path(__file__).with_name("rebase-panel-protection.py")
    (panel / "tools/rebase-panel-protection.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/protection-options.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-protection.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/protection-options.patch"'
    if script.count(before) != 1:
        raise ValueError("Cannot find pinned panel protection overlay")
    build.write_text(script.replace(before, after))


def configure_panel_cipher_rebase(panel):
    """Rebase explicit cipher support against a fork already tracking unsafe cipher warnings."""
    source = Path(__file__).with_name("rebase-panel-ciphers.py")
    (panel / "tools/rebase-panel-ciphers.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/explicit-cipher-suites.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-ciphers.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/explicit-cipher-suites.patch"'
    if script.count(before) != 1:
        raise ValueError("Cannot find pinned cipher suite overlay")
    build.write_text(script.replace(before, after))


def configure_panel_transport_rebase(panel):
    """Preserve the panel transport extensions on the current fork APIs."""
    source = Path(__file__).with_name("rebase-panel-transport.py")
    (panel / "tools/rebase-panel-transport.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/transport-balancer.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-transport.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/transport-balancer.patch"'
    if script.count(before) != 1:
        raise ValueError("Cannot find pinned transport/balancer overlay")
    build.write_text(script.replace(before, after))


def configure_panel_vless_reverse_rebase(panel):
    """Adapt reverse VLESS to the current fork without downgrading encryption."""
    source = Path(__file__).with_name("rebase-panel-vless-reverse.py")
    (panel / "tools/rebase-panel-vless-reverse.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/vless-reverse.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-vless-reverse.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-reverse.patch"'
    if script.count(before) != 1:
        raise ValueError("Cannot find pinned VLESS reverse overlay")
    build.write_text(script.replace(before, after))


def configure_panel_vless_handshake_rebase(panel):
    """Retain the upstream context-cancellable handshake implementation."""
    source = Path(__file__).with_name("rebase-panel-vless-handshake.py")
    (panel / "tools/rebase-panel-vless-handshake.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/vless-handshake.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-vless-handshake.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-handshake.patch"'
    if script.count(before) != 1:
        raise ValueError("Cannot find pinned VLESS handshake overlay")
    build.write_text(script.replace(before, after))


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


def refresh_awg_peer_source_check(lock):
    """Track the current AWG receiver name without dropping user statistics evidence."""
    path = "hiddify-sing-box/protocol/awg/endpoint.go"
    matches = [check for check in lock["source_checks"] if check.get("path") == path]
    if len(matches) != 1 or matches[0].get("contains") not in (
        "metadata.User = e.peerUsers.Lookup(source.Addr)",
        "metadata.User = receiver.peerUsers.Lookup(source.Addr)",
    ):
        raise ValueError("Unexpected AWG source evidence check")
    matches[0]["contains"] = "metadata.User = receiver.peerUsers.Lookup(source.Addr)"


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
    configure_panel_protection_rebase(panel)
    configure_panel_cipher_rebase(panel)
    configure_panel_transport_rebase(panel)
    configure_panel_vless_reverse_rebase(panel)
    # Refresh reverse tests copied from the maintained 3x-ui panel repository.
    # The new lifecycle requires a scope argument; adapter.V2RayServer no longer
    # exports Close(), and the fixture never starts that server.
    reverse_test = panel / "deploy/hiddify/reverse_runtime_test.go.in"
    reverse_source = reverse_test.read_text()
    old_start = "bridge.(adapter.Lifecycle).Start(adapter.StartStatePostStart)"
    old_close = "\tdefer api.Close()"
    if reverse_source.count(old_start) != 1 or reverse_source.count(old_close) != 1:
        raise ValueError("Reverse runtime tests changed, manual lifecycle review required")
    reverse_source = reverse_source.replace(old_start, "bridge.(adapter.Lifecycle).Start(adapter.StartStatePostStart, nil)")
    reverse_source = reverse_source.replace(old_close, "\t// The V2RayServer is not independently started; no Close lifecycle on interface.")
    reverse_test.write_text(reverse_source)
    configure_panel_vless_handshake_rebase(panel)

    lock_file = panel / "docs/hiddify-core-target.json"
    lock = json.loads(lock_file.read_text())
    refresh_go_module_source_check(lock, Path("go.mod").read_text())
    refresh_awg_peer_source_check(lock)
    refresh_panel_audit_toolchain_check(panel, lock)
    # The fork migrated NewRuleAction(ctx, logger, action) to
    # NewRuleAction(logger, action); preserve the panel sniff regression.
    sniff_test = panel / "deploy/hiddify/sniff_override_test.go.in"
    sniff_source = sniff_test.read_text()
    old = "NewRuleAction(context.Background(), nil, options)"
    if sniff_source.count(old) != 1 or '"context"' not in sniff_source:
        raise ValueError("Unexpected panel sniff test call signature")
    sniff_source = sniff_source.replace(old, "NewRuleAction(nil, options)")
    sniff_source = sniff_source.replace('    "context"\n', "")
    sniff_test.write_text(sniff_source)
    # mKCP is built into the newer fork behind its native feature tag.
    # Keep the panel's legacy interoperability tests and runtime in sync.
    tags = lock["panel_build"]["tags"]
    if "with_kcp" not in tags:
        tags.append("with_kcp")
    runtime = panel / "internal/hiddify/process.go"
    text = runtime.read_text()
    marker = "var requiredBuildTags = []string{"
    if text.count(marker) != 1:
        raise ValueError("Cannot find panel runtime build tags")
    line_start = text.index(marker)
    line_end = text.index("}", line_start)
    tag_line = text[line_start:line_end]
    if '"with_kcp"' not in tag_line:
        text = text[:line_end] + ', "with_kcp"' + text[line_end:]
        runtime.write_text(text)
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
