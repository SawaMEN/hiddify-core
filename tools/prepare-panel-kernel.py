#!/usr/bin/env python3
"""Prepare pinned native Linux kernel releases, never replaying panel patches."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

SHA = re.compile(r"^[0-9a-f]{40}$")
SUFFIX = re.compile(r"(-(?:mask|panel)[0-9]+(?:-ciphers[0-9]+)?(?:-net[0-9]+)?(?:-sec[0-9]+)?(?:-stream[0-9]+)?)$")
REQUIRED_TAGS = {"with_xui_panel", "with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_utls", "with_clash_api", "with_masque", "with_gvisor", "with_naive_outbound", "with_purego", "with_kcp", "http2legacy", "tfogo_checklinkname0"}
REQUIRED_FILES = {"cmd/cmd_xui_check.go", "hiddify-sing-box/internal/xuispeed/client.go", "hiddify-sing-box/transport/masque/server.go", "hiddify-sing-box/transport/masque/session.go", "hiddify-sing-box/protocol/masque/server.go", "hiddify-sing-box/protocol/vless/xui_reverse.go", "hiddify-sing-box/protocol/group/xui_balancer.go"}
NATIVE_MODULES = ("github.com/metacubex/utls", "github.com/starifly/sing-vmess", "github.com/xtls/xray-core")


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def gitlink(name):
    fields = command("git", "ls-tree", "HEAD", "--", name).split()
    if len(fields) != 4 or fields[:2] != ["160000", "commit"] or fields[3] != name or not SHA.fullmatch(fields[2]):
        raise ValueError("Missing pinned native gitlink: " + name)
    return fields[2]


def patch_profile_suffix(version):
    match = SUFFIX.search(version) if isinstance(version, str) else None
    if match is None:
        raise ValueError("Unreviewed kernel compatibility suffix")
    return match.group(1)


def release_tag(core_sha, sing_sha, channel, suffix):
    if not SHA.fullmatch(core_sha) or not SHA.fullmatch(sing_sha) or channel not in ("dev", "stable"):
        raise ValueError("Invalid release source identity")
    prefix = "sawamen" if channel == "stable" else "sawamen-dev"
    return f"{prefix}-{core_sha[:12]}-{sing_sha[:12]}{suffix}"


def verify_native_panel(panel, lock):
    build = lock.get("panel_build")
    if not isinstance(build, dict):
        raise ValueError("Missing native panel contract")
    tags = build.get("tags")
    files = build.get("native_source_files")
    if not isinstance(tags, list) or not REQUIRED_TAGS.issubset(tags):
        raise ValueError("Native kernel feature tags missing")
    if not isinstance(files, list) or len(files) < 20 or len(files) != len(set(files)) or not REQUIRED_FILES.issubset(files):
        raise ValueError("Missing native source evidence")
    if any(key.endswith("_overlay") for key in build) or "speed_broker_sdk" in build:
        raise ValueError("A retired panel overlay remains in the manifest")
    checks = {row["path"]: row["contains"] for row in lock.get("source_checks", [])}
    if checks.get("cmd/cmd_xui_check.go") != 'Use:   "check"' or checks.get("hiddify-sing-box/transport/masque/server.go") != 'xuispeed.New("masque:"':
        raise ValueError("Native CLI or speed-broker evidence missing")
    builder = (panel / "tools/build-hiddify-core.sh").read_text()
    if ".patch" in builder or ".go.in" in builder or "xui-dependencies" in builder:
        raise ValueError("Native builder still modifies upstream sources")
    for evidence in ("verify-hiddify-build.py", "native_source_files", "go mod tidy", "go mod download", "go build -buildvcs=true", "sha256sum"):
        if evidence not in builder:
            raise ValueError("Missing native release gate: " + evidence)
    return build


def prepare(panel):
    core_sha, channel = os.environ["KERNEL_COMMIT"], os.environ["KERNEL_CHANNEL"]
    if not SHA.fullmatch(core_sha) or command("git", "rev-parse", "HEAD") != core_sha:
        raise ValueError("Kernel checkout differs from trigger")
    if channel not in ("stable", "dev"):
        raise ValueError("Invalid kernel channel")
    sing_gitlink = gitlink("hiddify-sing-box")
    sing_sha = os.environ.get("SINGBOX_COMMIT") or sing_gitlink
    ray_sha = gitlink("ray2sing")
    if not SHA.fullmatch(sing_sha) or (channel == "stable" and sing_sha != sing_gitlink):
        raise ValueError("Stable release must use reviewed Sing-box gitlink")
    lock_path = panel / "deploy/hiddify/hiddify-core-target.json"
    lock = json.loads(lock_path.read_text())
    if lock.get("schema_version") != 2 or lock.get("stage") != 4:
        raise ValueError("Unexpected native panel lock schema")
    build = verify_native_panel(panel, lock)
    target = lock["target"]
    if target["repository"] != "https://github.com/SawaMEN/hiddify-core":
        raise ValueError("Wrong kernel origin")
    if target["submodule_repositories"].get("hiddify-sing-box") != "https://github.com/SawaMEN/hiddify-sing-box" or target["submodule_repositories"].get("ray2sing") != "https://github.com/SawaMEN/ray2sing":
        raise ValueError("Wrong pinned native module origins")
    module = Path("go.mod").read_text()
    match = re.search(r"(?m)^go ([0-9]+\.[0-9]+(?:\.[0-9]+)?)$", module)
    if match is None or "go" + match.group(1) != "go1.27.3":
        raise ValueError("Unreviewed core Go toolchain")
    if not Path("cmd/cmd_xui_check.go").is_file():
        raise ValueError("Native raw config check command missing")

    version = release_tag(core_sha, sing_sha, channel, patch_profile_suffix(target["version"]))
    target["version"], target["commit"] = version, core_sha
    target["submodules"]["hiddify-sing-box"] = sing_sha
    target["submodules"]["ray2sing"] = ray_sha
    build["toolchain"] = "go" + match.group(1)
    build["sing_box_version"] = "sawamen-" + sing_sha[:12]
    build["protection_dependency_versions"] = {name: "native:" + sing_sha for name in NATIVE_MODULES}
    lock_path.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n")
    profile_file = panel / "internal/hiddify/translate.go"
    profile = profile_file.read_text()
    for name, value in (("TargetVersion", version), ("TargetRevision", core_sha)):
        pattern = r'(?m)^([ \t]*' + name + r'[ \t]*=[ \t]*)"[^"\n]+"'
        profile, count = re.subn(pattern, lambda m: m.group(1) + '"' + value + '"', profile)
        if count != 1:
            raise ValueError("Cannot update panel identity: " + name)
    profile_file.write_text(profile)
    print(f"Native release: core={core_sha} sing-box={sing_sha} parser={ray_sha} tag={version}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: prepare-panel-kernel.py PANEL_CHECKOUT")
    prepare(Path(sys.argv[1]))
