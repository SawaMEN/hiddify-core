#!/usr/bin/env python3
"""Prepare a reproducible panel kernel build for the current core git commit.

The core release workflow checks out SawaMEN/3x-ui for overlays, then rewrites
only this disposable checkout's manifest and expected process identity.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import runpy
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


def configure_panel_masque_rebase(panel):
    """Keep per-user MASQUE rate limits on the new batched-packet transport."""
    patch = panel / "deploy/speed/masque.patch"
    old_patch = patch.read_text()
    for evidence in ("current.speed, err = brokerclient.New", "waitDownload", "SpeedScope"):
        if evidence not in old_patch:
            raise ValueError("Panel MASQUE limiter changed; manual rebase required")
    source = Path(__file__).with_name("rebase-panel-masque.py")
    (panel / "tools/rebase-panel-masque.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/speed/masque.patch"'
    after = 'python3 "$task_root/tools/rebase-panel-masque.py" "$task_source/core/hiddify-sing-box"'
    if script.count(before) != 1:
        raise ValueError("Cannot find panel MASQUE speed limiter overlay")
    build.write_text(script.replace(before, after))


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


def refresh_native_protection_verifier(panel, native_versions):
    """Keep the panel's manifest-derived source check; adapt only legacy snapshots."""
    verifier_file = panel / "tools/verify-hiddify-build.py"
    verifier = verifier_file.read_text()
    dynamic = 'require(build["protection_dependency_versions"] == native_pins,'
    declaration = 'native_pins = {module: "native:" + native_sha for module in ('
    if dynamic in verifier:
        if verifier.count(dynamic) != 1 or declaration not in verifier:
            raise ValueError("Native protection source audit changed unexpectedly")
        return  # The manifest-derived assertion already follows the current gitlink.
    marker = 'require(build["protection_dependency_versions"] == '
    matches = [line for line in verifier.splitlines() if marker in line]
    if len(matches) != 1:
        raise ValueError("Native protection verifier missing or ambiguous")
    verified = ('    require(build["protection_dependency_versions"] == ' +
                repr(native_versions) + ', "Pinned native modules differ from verified Sing-box")')
    verifier_file.write_text(verifier.replace(matches[0], verified))


def prepare_runtime_dependencies(panel, module):
    """Require reviewed dependencies in the native module, without a no-op patch."""
    required = (
        "github.com/apernet/quic-go",
        "github.com/juju/ratelimit",
        "github.com/xtls/reality",
    )
    for name in required:
        if not re.search(r"(?m)^\s*" + re.escape(name) + r"\s+v\S+", module):
            raise ValueError("Native core is missing reviewed dependency: " + name)
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    old = 'git -C "$task_source/core" apply "$task_root/deploy/hiddify/runtime-dependencies.patch"'
    new = ': # reviewed runtime dependencies are already in native core/go.mod'
    if script.count(old) == 1 and new not in script:
        build.write_text(script.replace(old, new, 1))
    elif script.count(new) != 1 or old in script:
        raise ValueError("Unexpected native core dependency integration point")
    # No overlay file is read or applied, even with an older panel checkout.


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


def refresh_xdrive_source_check(lock):
    """Retarget XDrive evidence to its constructor registration after the transport split."""
    old_path = "hiddify-sing-box/transport/v2ray/transport.go"
    new_path = "hiddify-sing-box/transport/v2rayxdrive/transport.go"
    old_evidence = "return xdrive.NewServer"
    new_evidence = "v2ray.RegisterXDriveConstructor(NewServer, NewClient)"
    matches = [
        check for check in lock["source_checks"]
        if check.get("contains") in (old_evidence, new_evidence)
    ]
    if len(matches) != 1:
        raise ValueError("Missing or duplicate XDrive runtime source evidence")
    check = matches[0]
    if (check.get("path"), check.get("contains")) not in (
        (old_path, old_evidence), (new_path, new_evidence),
    ):
        raise ValueError("Unexpected XDrive runtime source evidence")
    check["path"] = new_path
    check["contains"] = new_evidence


def refresh_panel_verifier_toolchain_check(panel, lock):
    """Update the actual panel runtime verifier for the checked-out core Go version."""
    toolchain = lock["panel_build"]["toolchain"]
    if not re.fullmatch(r"go[0-9]+\.[0-9]+(?:\.[0-9]+)?", toolchain):
        raise ValueError("Invalid runtime audit toolchain")
    verifier_file = panel / "tools/verify-hiddify-build.py"
    verifier = verifier_file.read_text()
    check = r'require\(build\["toolchain"\] == "go[0-9]+\.[0-9]+(?:\.[0-9]+)?", "Runtime must use the (?:pinned security-patched toolchain|reviewed security-patched core toolchain)"\)'
    updated, count = re.subn(
        check,
        'require(build["toolchain"] == "' + toolchain + '", "Runtime must use the reviewed security-patched core toolchain")',
        verifier,
    )
    if count != 1:
        raise ValueError("Cannot update panel runtime audit toolchain check")
    verifier_file.write_text(updated)


def patch_profile_suffix(version):
    """Preserve known numbered panel/mask and optional cipher profiles."""
    if not isinstance(version, str):
        raise ValueError("Invalid patch profile")
    match = re.search(r"(-(?:panel|mask)[0-9]+(?:-ciphers[0-9]+)?(?:-net[0-9]+)?(?:-sec[0-9]+)?(?:-stream[0-9]+)?)$", version)
    if not match:
        raise ValueError("Cannot determine patch profile (expected -panelN or -maskN with known suffixes)")
    return match.group(1)






def configure_panel_utls_checkout(panel, module):
    """Keep the panel's uTLS overlay on the same fork as the native core."""
    if "replace github.com/metacubex/utls => ./hiddify-sing-box/replace/utls-native" in module.splitlines():
        return  # Reviewed module pinned inside the Sing-box gitlink; never patch it twice.
    if not re.search(
        r"(?m)^replace github\.com/metacubex/utls => github\.com/Leadaxe/utls-lx v[^\s]+$",
        module,
    ):
        raise ValueError("Missing pinned native uTLS replacement")
    script_path = panel / "tools/build-hiddify-core.sh"
    script = script_path.read_text()
    old_download = "go mod download github.com/metacubex/utls@v1.8.8"
    new_download = "go mod download github.com/metacubex/utls"
    if script.count(old_download) != 1:
        raise ValueError("Unexpected old uTLS download command")
    script = script.replace(old_download, new_download)
    old_copy = 'cp -a "$task_module_cache/github.com/metacubex/utls@v1.8.8" "$task_source/core/xui-dependencies/utls"'
    new_copy = (
        'task_utls_source=$(cd "$task_source/core"; '
        'go list -m -f \'{{if .Replace}}{{.Replace.Dir}}{{else}}{{.Dir}}{{end}}\' github.com/metacubex/utls)\n'
        'test -n "$task_utls_source" && test -f "$task_utls_source/go.mod"\n'
        'cp -a "$task_utls_source" "$task_source/core/xui-dependencies/utls"'
    )
    if script.count(old_copy) != 1:
        raise ValueError("Unexpected old uTLS source copy command")
    script_path.write_text(script.replace(old_copy, new_copy))


def configure_panel_owned_parser(panel, lock):
    """Keep the core's pinned, compatible owned ray2sing revision and origin."""
    repos = lock["target"]["submodule_repositories"]
    if repos.get("ray2sing") not in {
        "https://github.com/hiddify/ray2sing",
        "https://github.com/SawaMEN/ray2sing",
    }:
        raise ValueError("Unexpected parser origin in panel lock")
    repos["ray2sing"] = "https://github.com/SawaMEN/ray2sing"
    script_path = panel / "tools/build-hiddify-core.sh"
    script = script_path.read_text()
    old = 'git -C "$task_source/core" config submodule.ray2sing.url https://github.com/hiddify/ray2sing.git'
    new = 'git -C "$task_source/core" config submodule.ray2sing.url https://github.com/SawaMEN/ray2sing.git'
    if script.count(old) != 1:
        raise ValueError("Parser submodule origin override moved")
    script_path.write_text(script.replace(old, new))


def configure_panel_kcp_finalmask_interop(panel):
    """Verify native KCP regressions; never inject the deleted API fixture."""
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    obsolete_copy = (
        'cp "$task_root/deploy/hiddify/kcp_transport_test.go.in" '
        '"$task_source/core/hiddify-sing-box/transport/v2raykcp/xui_kcp_test.go"'
    )
    native_checks = (
        'test -f "$task_source/core/hiddify-sing-box/transport/v2raykcp/h_kcp_test.go"\n'
        'test -f "$task_source/core/hiddify-sing-box/h_xray_kcp_test.go"'
    )
    if script.count(obsolete_copy) == 1 and native_checks not in script:
        script = script.replace(obsolete_copy, native_checks, 1)
    elif script.count(native_checks) != 1 or obsolete_copy in script:
        raise ValueError("Native KCP source checks are missing or ambiguous")
    old_run = ("github.com/sagernet/sing-box/transport/v2raykcp "
               "github.com/sagernet/sing-box/transport/v2rayxhttp "
               "github.com/sagernet/sing-box/protocol/group -run '^TestXUIPanel'")
    new_run = old_run.replace("-run '^TestXUIPanel'", "-run '^Test(XUIPanel|H_KCP)'")
    if script.count(old_run) == 1 and new_run not in script:
        script = script.replace(old_run, new_run, 1)
    elif script.count(new_run) != 1 or old_run in script:
        raise ValueError("Native KCP integration test invocation is missing")
    original = (
        'CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test '
        '-ldflags "$task_test_link_flags" -tags "$task_test_tags" '
        "github.com/sagernet/sing-box -run '^TestXUIPanelInboundFinalMask' -count=1 -timeout=60s"
    )
    native = (
        'CGO_ENABLED=0 GOTOOLCHAIN="${target[5]}" go test '
        '-ldflags "$task_test_link_flags" -tags "$task_test_tags" '
        "github.com/sagernet/sing-box -run '^TestH_(XrayKCP|KCPWithTLS)$' -count=1 -timeout=180s"
    )
    if script.count(original) != 1:
        raise ValueError("Missing panel FinalMask native regression")
    if script.count(native) == 0:
        script = script.replace(original, native + "\n    " + original, 1)
    elif script.count(native) != 1:
        raise ValueError("Duplicate KCP native regression")
    build.write_text(script)


def configure_panel_inbound_finalmask_rebase(panel):
    """Replace the removed TCP mask listener API after applying the panel overlay."""
    source = Path(__file__).with_name("rebase-panel-inbound-finalmask.py")
    (panel / "tools/rebase-panel-inbound-finalmask.py").write_text(source.read_text())
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    before = 'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/inbound-finalmask.patch"'
    if script.count(before) != 1:
        raise ValueError("Missing panel inbound FinalMask overlay")
    after = before + '\npython3 "$task_root/tools/rebase-panel-inbound-finalmask.py" "$task_source/core/hiddify-sing-box"'
    build.write_text(script.replace(before, after))


def configure_panel_xray_dependency(panel, module):
    """Use the Xray revision required by the current core, not the panel's stale copy."""
    match = re.search(r"(?m)^\s*github\.com/xtls/xray-core\s+(v[^\s]+)", module)
    if match is None:
        raise ValueError("Missing core Xray version")
    version = match.group(1)
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    old_version = "v1.260327.1-0.20260728075948-5ca6f4b7d4dc"
    previous = "github.com/xtls/xray-core@" + old_version
    if script.count(previous) != 2:
        raise ValueError("Unexpected panel Xray download and copy paths")
    build.write_text(script.replace(previous, "github.com/xtls/xray-core@" + version))
    verifier_file = panel / "tools/verify-hiddify-build.py"
    verifier = verifier_file.read_text()
    if verifier.count(old_version) == 1:
        # Legacy audited panels retained a literal Xray version.
        verifier_file.write_text(verifier.replace(old_version, version))
    elif verifier.count(old_version) == 0:
        # Native panels take the reviewed Xray module from the fork itself.
        if 'build["protection_dependency_versions"]' not in verifier or "native_pins" not in verifier:
            raise ValueError("Native Xray provenance audit is missing")
    else:
        raise ValueError("Ambiguous Xray pin in panel runtime audit")

    # Xray 26.10 implemented the old mux fixes natively: PacketReader now
    # retains the destination by value, and SessionManager.Allocate installs
    # stream endpoints while holding its mutex. Applying the old mux patch
    # would conflict with that refactor and regress its lifecycle handling.
    if version != old_version:
        if version != "v1.260327.1-0.20261031132019-a4a3dff5b20c":
            raise ValueError("Unreviewed Xray upgrade: rebase runtime compatibility patch")
        patch = panel / "deploy/hiddify/xray-runtime-compat.patch"
        fragments = re.split(r"(?=^--- (?:a/|/dev/null))", patch.read_text(), flags=re.MULTILINE)
        obsolete = {"common/mux/reader.go", "common/mux/client.go"}
        filtered = []
        removed = set()
        for fragment in fragments:
            match = re.match(r"--- a/([^\n]+)\n", fragment)
            if match and match.group(1) in obsolete:
                if match.group(1) in removed:
                    raise ValueError("Duplicate Xray mux patch section")
                removed.add(match.group(1))
                continue
            filtered.append(fragment)
        if removed != obsolete:
            raise ValueError("Unexpected Xray mux patch sections")
        patch.write_text("".join(filtered))
    return version



def configure_native_overlay_canary(panel):
    """Probe the native sing-box candidate without reapplying completed overlays.

    Only the isolated draft core PR uses this; the published 3x-ui builder is
    unchanged. Every disabled overlay requires a native source regression check.
    Remaining incomplete overlays and all security verification still execute.
    """
    build = panel / "tools/build-hiddify-core.sh"
    script = build.read_text()
    native = (
        ('git -C "$task_source/core" apply "$task_root/deploy/hiddify/clash-user.patch"',
         "experimental/clashapi/trafficontrol/xui_panel_test.go"),
        ('git -C "$task_source/core" apply "$task_root/deploy/hiddify/sniff-override.patch"',
         "route/rule/xui_panel_sniff_test.go"),
        ('git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/http1-options.patch"',
         "option/xui_panel_http1_test.go"),
        ('git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/hysteria-safety.patch"',
         "protocol/hysteria2/inbound.go"),
        ('git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/inbound-finalmask.patch"',
         "xui_inbound_finalmask_test.go"),
        ('python3 "$task_root/tools/rebase-panel-ciphers.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/explicit-cipher-suites.patch"',
         "common/tls/xui_explicit_ciphers_test.go"),
        ('git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/stream-handshake.patch"',
         "transport/trojan/xui_stream_test.go"),
        ('python3 "$task_root/tools/rebase-panel-vless-handshake.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-handshake.patch"',
         "protocol/vless/h_handshake_close_once_test.go"),
    )
    for application, regression in native:
        if script.count(application) != 1:
            raise ValueError("Unexpected native overlay application: " + regression)
        check = (
            'test -f "$task_source/core/hiddify-sing-box/' + regression +
            '" || { echo "Missing native regression: ' + regression + '" >&2; exit 1; }'
        )
        script = script.replace(application, check)
    # The old post-apply API adapter would modify already-native FinalMask.
    old_rebase = 'python3 "$task_root/tools/rebase-panel-inbound-finalmask.py" "$task_source/core/hiddify-sing-box"'
    if script.count(old_rebase) != 1:
        raise ValueError("FinalMask legacy rebase moved")
    script = script.replace(old_rebase, ': # native FinalMask; no legacy rebase')
    build.write_text(script)

def configure_native_http2_extended_connect(panel, lock):
    """Keep MASQUE HTTP/2 extended CONNECT functional with Go 1.27.

    The stdlib-backed HTTP/2 wrapper rejects the :protocol pseudo-header
    on this pinned toolchain. x/net/http2's reviewed http2legacy transport
    retains the extended CONNECT implementation used by MASQUE and XHTTP.
    This tag must describe the actual binary and be verified on startup.
    """
    tags = lock["panel_build"]["tags"]
    if not isinstance(tags, list) or not all(isinstance(tag, str) for tag in tags):
        raise ValueError("Invalid reviewed native feature tags")
    if "http2legacy" not in tags:
        tags.append("http2legacy")
    runtime_file = panel / "internal/hiddify/process.go"
    runtime = runtime_file.read_text()
    marker = "var requiredBuildTags = []string{"
    if runtime.count(marker) != 1:
        raise ValueError("Missing panel required build tags")
    start = runtime.index(marker)
    end = runtime.index("}", start)
    tags_text = runtime[start:end]
    if '"http2legacy"' not in tags_text:
        runtime_file.write_text(runtime[:end] + ', "http2legacy"' + runtime[end:])


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

    module = Path("go.mod").read_text()
    prepare_runtime_dependencies(panel, module)
    xray_version = configure_panel_xray_dependency(panel, module)
    configure_panel_utls_checkout(panel, module)
    configure_panel_inbound_finalmask_rebase(panel)
    configure_panel_kcp_finalmask_interop(panel)
    configure_panel_masque_rebase(panel)
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
    configure_native_overlay_canary(panel)
    # Only native modules from the reviewed Sing-box gitlink may replace overlays.
    for entry in (
        "replace github.com/sagernet/sing-vmess => ./hiddify-sing-box/third_party/sing-vmess",
        "replace github.com/metacubex/utls => ./hiddify-sing-box/replace/utls-native",
        "replace github.com/xtls/xray-core => ./hiddify-sing-box/third_party/xray-core",
    ):
        if module.splitlines().count(entry) != 1:
            raise ValueError("Missing pinned native module: " + entry)
    runpy.run_path(str(Path(__file__).with_name("native_overlay_prepare.py")))["rewrite"](panel)

    lock_file = panel / "deploy/hiddify/hiddify-core-target.json"
    lock = json.loads(lock_file.read_text())
    configure_panel_owned_parser(panel, lock)
    go_version = re.search(r"(?m)^go (\d+\.\d+(?:\.\d+)?)$", module)
    if go_version is None:
        raise ValueError("Missing core Go toolchain")
    lock["panel_build"]["toolchain"] = "go" + go_version.group(1)
    native_versions = {name: "native:" + sing_sha for name in (
        "github.com/metacubex/utls", "github.com/starifly/sing-vmess", "github.com/xtls/xray-core"
    )}
    lock["panel_build"]["protection_dependency_versions"] = native_versions
    refresh_native_protection_verifier(panel, native_versions)
    refresh_go_module_source_check(lock, module)
    refresh_awg_peer_source_check(lock)
    refresh_xdrive_source_check(lock)
    refresh_panel_verifier_toolchain_check(panel, lock)
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
    # Use the x/net HTTP/2 transport while Go 1.27's stdlib wrapper
    # cannot handle MASQUE extended CONNECT's :protocol pseudo-header.
    configure_native_http2_extended_connect(panel, lock)
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
