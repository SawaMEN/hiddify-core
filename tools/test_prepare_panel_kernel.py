import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare-panel-kernel.py"))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class HTTP2LegacyBuildTest(unittest.TestCase):
    def test_requires_legacy_http2_transport_in_runtime_and_release_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            panel = Path(tmp)
            runtime = panel / "internal/hiddify/process.go"
            runtime.parent.mkdir(parents=True)
            runtime.write_text('var requiredBuildTags = []string{"with_kcp"}\n')
            lock = {"panel_build": {"tags": ["with_kcp", "with_purego"]}}
            prepare.configure_native_http2_extended_connect(panel, lock)
            prepare.configure_native_http2_extended_connect(panel, lock)
            self.assertEqual(1, lock["panel_build"]["tags"].count("http2legacy"))
            self.assertEqual(1, runtime.read_text().count('"http2legacy"'))
            self.assertIn('"with_kcp"', runtime.read_text())

    def test_rejects_missing_runtime_tag_contract(self):
        with tempfile.TemporaryDirectory() as tmp:
            panel = Path(tmp)
            runtime = panel / "internal/hiddify/process.go"
            runtime.parent.mkdir(parents=True)
            runtime.write_text("invalid runtime")
            with self.assertRaises(ValueError):
                prepare.configure_native_http2_extended_connect(
                    panel, {"panel_build": {"tags": ["with_kcp"]}}
                )


class ReleaseProfileTest(unittest.TestCase):
    def test_accepts_current_mask_profile(self):
        self.assertEqual("-mask1", prepare.patch_profile_suffix(
            "sawamen-b310097c8e7a-338f0c9457a4-mask1"))

    def test_preserves_combined_mask_and_cipher_profile(self):
        self.assertEqual("-mask1-ciphers1", prepare.patch_profile_suffix(
            "sawamen-b310097c8e7a-338f0c9457a4-mask1-ciphers1"))

    def test_preserves_legacy_panel_with_cipher_profile(self):
        self.assertEqual("-panel9-ciphers2", prepare.patch_profile_suffix(
            "sawamen-3d4ee155c924-244e333149ec-panel9-ciphers2"))

    def test_preserves_current_security_stream_mask_profile(self):
        current = "sawamen-b310097c8e7a-338f0c9457a4-mask1-ciphers1-net62-sec1-stream1"
        self.assertEqual("-mask1-ciphers1-net62-sec1-stream1", prepare.patch_profile_suffix(current))

    def test_retains_legacy_panel_profile(self):
        self.assertEqual("-panel9", prepare.patch_profile_suffix(
            "sawamen-3d4ee155c924-244e333149ec-panel9"))

    def test_rejects_missing_or_arbitrary_profiles(self):
        for version in ("sawamen-abc-def", "sawamen-abc-def-unknown1", "sawamen-abc-def-panel1-extra", "sawamen-abc-def-mask1-ciphers1-extra", "sawamen-abc-def-mask1-ciphers", "", None):
            with self.subTest(version=version), self.assertRaises(ValueError):
                prepare.patch_profile_suffix(version)



class ModuleSourceCheckTest(unittest.TestCase):
    def test_rebases_stale_toolchain_evidence(self):
        lock = {"panel_build": {"toolchain": "go1.26.9"}, "source_checks": [
            {"path": "go.mod", "contains": "go 1.26.8"},
            {"path": "cmd/cmd_sing_run.go", "contains": "srun"},
        ]}
        prepare.refresh_go_module_source_check(
            lock, "module example.com/core\n\ngo 1.26.10\n\ngodebug multipathtcp=0\n"
        )
        self.assertEqual("go 1.26.10", lock["source_checks"][0]["contains"])
        self.assertEqual("go1.26.10", lock["panel_build"]["toolchain"])
        self.assertEqual("srun", lock["source_checks"][1]["contains"])

    def test_rejects_malformed_or_missing_go_evidence(self):
        for checks in ([], [{"path": "go.mod", "contains": "anything"}],
                       [{"path": "go.mod", "contains": "go 1.25"},
                        {"path": "go.mod", "contains": "go 1.26"}]):
            with self.subTest(checks=checks), self.assertRaises(ValueError):
                prepare.refresh_go_module_source_check(
                    {"panel_build": {"toolchain": "go1.26.9"}, "source_checks": checks}, "module example.com/core\ngo 1.26.10\n"
                )

    def test_rejects_missing_go_declaration(self):
        with self.assertRaises(ValueError):
            prepare.refresh_go_module_source_check(
                {"panel_build": {"toolchain": "go1.26.9"}, "source_checks": [{"path": "go.mod", "contains": "go 1.26.8"}]},
                "module example.com/core\n",
            )


    def test_rejects_invalid_pinned_toolchain(self):
        with self.assertRaises(ValueError):
            prepare.refresh_go_module_source_check(
                {"panel_build": {"toolchain": "invalid"}, "source_checks": [
                    {"path": "go.mod", "contains": "go 1.26.8"}
                ]}, "module example.com/core\ngo 1.26.10\n",
            )


class PanelVerifierToolchainTest(unittest.TestCase):
    def test_rebases_exact_verifier_pin_to_core_toolchain(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            audit = folder / "tools/verify-hiddify-build.py"
            audit.parent.mkdir()
            audit.write_text('require(build["toolchain"] == "go1.26.9", "Runtime must use the pinned security-patched toolchain")\\n')
            prepare.refresh_panel_verifier_toolchain_check(folder, {"panel_build": {"toolchain": "go1.26.10"}})
            self.assertIn('build["toolchain"] == "go1.26.10"', audit.read_text())
            self.assertNotIn('build["toolchain"] == "go1.26.9"', audit.read_text())

    def test_refuses_unknown_verifier_check_or_toolchain(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            audit = folder / "tools/verify-hiddify-build.py"
            audit.parent.mkdir()
            audit.write_text("unknown runtime audit")
            with self.assertRaises(ValueError):
                prepare.refresh_panel_verifier_toolchain_check(folder, {"panel_build": {"toolchain": "go1.26.10"}})
            with self.assertRaises(ValueError):
                prepare.refresh_panel_verifier_toolchain_check(folder, {"panel_build": {"toolchain": "unknown"}})


class AWGSourceEvidenceTest(unittest.TestCase):
    def test_current_receiver_name_keeps_peer_statistics_check(self):
        lock = {"source_checks": [{"path": "hiddify-sing-box/protocol/awg/endpoint.go",
                                  "contains": "metadata.User = e.peerUsers.Lookup(source.Addr)"}]}
        prepare.refresh_awg_peer_source_check(lock)
        self.assertEqual(
            "metadata.User = receiver.peerUsers.Lookup(source.Addr)",
            lock["source_checks"][0]["contains"],
        )

    def test_rejects_unverified_awg_source_evidence(self):
        with self.assertRaises(ValueError):
            prepare.refresh_awg_peer_source_check({"source_checks": []})


class XDriveSourceEvidenceTest(unittest.TestCase):
    def test_migrates_xdrive_check_to_registered_transport(self):
        lock = {"source_checks": [
            {"path": "hiddify-sing-box/transport/v2ray/transport.go",
             "contains": "return xhttp.NewServer"},
            {"path": "hiddify-sing-box/transport/v2ray/transport.go",
             "contains": "return xdrive.NewServer"},
        ]}
        prepare.refresh_xdrive_source_check(lock)
        self.assertEqual(
            {"path": "hiddify-sing-box/transport/v2rayxdrive/transport.go",
             "contains": "v2ray.RegisterXDriveConstructor(NewServer, NewClient)"},
            lock["source_checks"][1],
        )
        self.assertEqual("return xhttp.NewServer", lock["source_checks"][0]["contains"])
        prepare.refresh_xdrive_source_check(lock)  # idempotent

    def test_rejects_missing_or_ambiguous_source_evidence(self):
        with self.assertRaises(ValueError):
            prepare.refresh_xdrive_source_check({"source_checks": []})
        with self.assertRaises(ValueError):
            prepare.refresh_xdrive_source_check({"source_checks": [
                {"path": "hiddify-sing-box/transport/v2ray/transport.go",
                 "contains": "return xdrive.NewServer"},
                {"path": "hiddify-sing-box/transport/v2rayxdrive/transport.go",
                 "contains": "v2ray.RegisterXDriveConstructor(NewServer, NewClient)"},
            ]})


class NativeDependenciesTest(unittest.TestCase):
    def fixture(self, folder):
        panel = folder / "panel"
        (panel / "tools").mkdir(parents=True)
        (panel / "tools/build-hiddify-core.sh").write_text(
            'git -C "$task_source/core" apply "$task_root/deploy/hiddify/runtime-dependencies.patch"\n')
        return panel

    def module(self):
        return "module core\nrequire (\n\tgithub.com/apernet/quic-go v0.59.0\n\tgithub.com/juju/ratelimit v1.0.2\n\tgithub.com/xtls/reality v1.0.0\n)\n"

    def test_native_dependencies_eliminate_legacy_patch(self):
        with tempfile.TemporaryDirectory() as tmp:
            panel = self.fixture(Path(tmp))
            prepare.prepare_runtime_dependencies(panel, self.module())
            script = (panel / "tools/build-hiddify-core.sh").read_text()
            self.assertIn("reviewed runtime dependencies are already", script)
            self.assertNotIn("runtime-dependencies.patch", script)
            prepare.prepare_runtime_dependencies(panel, self.module())

    def test_missing_native_dependency_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            panel = self.fixture(Path(tmp))
            with self.assertRaises(ValueError):
                prepare.prepare_runtime_dependencies(panel, self.module().replace(
                    "github.com/juju/ratelimit", "github.com/absent/dependency"))



class NativeOverlayCanaryTest(unittest.TestCase):
    def test_native_gates_preserve_incomplete_overlays(self):
        applications = [
            'git -C "$task_source/core" apply "$task_root/deploy/hiddify/clash-user.patch"',
            'git -C "$task_source/core" apply "$task_root/deploy/hiddify/sniff-override.patch"',
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/http1-options.patch"',
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/hysteria-safety.patch"',
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/inbound-finalmask.patch"',
            'python3 "$task_root/tools/rebase-panel-ciphers.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/explicit-cipher-suites.patch"',
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/stream-handshake.patch"',
            'python3 "$task_root/tools/rebase-panel-vless-handshake.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-handshake.patch"',
            'python3 "$task_root/tools/rebase-panel-inbound-finalmask.py" "$task_source/core/hiddify-sing-box"',
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/hiddify/transport-balancer.patch"',
        ]
        with tempfile.TemporaryDirectory() as tmp:
            panel = Path(tmp)
            script = panel / "tools/build-hiddify-core.sh"
            script.parent.mkdir(parents=True)
            source = "\n".join(applications) + "\n"
            script.write_text(source)
            prepare.configure_native_overlay_canary(panel)
            generated = script.read_text()
            self.assertEqual(8, generated.count("Missing native regression:"))
            self.assertNotIn("rebase-panel-inbound-finalmask.py", generated)
            self.assertIn("transport-balancer.patch", generated)
            self.assertIn("test -f", generated)
            # A changed overlay application cannot silently disappear.
            script.write_text(source.replace("clash-user.patch", "renamed-clash.patch"))
            with self.assertRaises(ValueError):
                prepare.configure_native_overlay_canary(panel)
            self.assertEqual(source.replace("clash-user.patch", "renamed-clash.patch"), script.read_text())



class NativeOverlayRewriteTest(unittest.TestCase):
    """Ensure all staged native module overlays disappear without dropping security gates."""

    def fixture(self):
        commands = [
            'git -C "$task_source/core/hiddify-sing-box" apply "$task_root/deploy/speed/masque.patch"',
            '# Modify verified dependency copies, never the shared Go module cache.',
            'git -C "$task_source/core" apply --directory=xui-dependencies/utls "$task_root/deploy/hiddify/utls-protection.patch"',
            'git -C "$task_source/core" apply --directory=xui-dependencies/sing-vmess "$task_root/deploy/hiddify/sing-vmess-protection.patch"',
            'python3 "$task_root/tools/rebase-panel-protection.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/protection-options.patch"',
            'python3 "$task_root/tools/rebase-panel-transport.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/transport-balancer.patch"',
            'python3 "$task_root/tools/rebase-panel-vless-reverse.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-reverse.patch"',
            'git -C "$task_source/core" apply --directory=xui-dependencies/sing-vmess "$task_root/deploy/hiddify/sing-vmess-reverse.patch"',
            'git -C "$task_source/core" apply --directory=xui-dependencies/xray-core "$task_root/deploy/hiddify/xray-runtime-compat.patch"',
        ]
        fixtures = (
            'inbound_finalmask', 'http1_options', 'clash_user', 'sniff_override',
            'stream_upgrade', 'stream_trojan', 'explicit_cipher_suites',
            'vless_handshake', 'reverse_vision', 'reverse_runtime',
            'xhttp_extension', 'balancer_strategy', 'reality_protection',
            'vision_preconnect', 'vision_padding',
        )
        for name in fixtures:
            commands.append('cp -- "$task_root/deploy/hiddify/' + name +
                            '_test.go.in" "$task_source/core/hiddify-sing-box/placeholder.go"')
        commands += [
            '  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/metacubex/utls=./xui-dependencies/utls -replace github.com/sagernet/sing-vmess=./xui-dependencies/sing-vmess',
            '  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/xtls/xray-core=./xui-dependencies/xray-core',
            '  GOTOOLCHAIN="${target[5]}" go mod download "${security_dependencies[@]}"',
            'python3 "$task_root/tools/verify-hiddify-build.py" --upstream "$task_source/core"',
            'python3 "$task_root/tools/rebase-panel-masque.py" "$task_source/core/hiddify-sing-box"',
            'go mod tidy',
            'go build -buildvcs=true',
        ]
        return "\n".join(commands) + "\n"

    def rewrite_fixture(self, fixture):
        import runpy
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            build = folder / "tools/build-hiddify-core.sh"
            build.parent.mkdir()
            build.write_text(fixture)
            result = runpy.run_path(str(Path(__file__).with_name("native_overlay_prepare.py")))["rewrite"](folder)
            return result, build.read_text()

    def test_rewrite_uses_native_modules_and_keeps_security_checks(self):
        count, result = self.rewrite_fixture(self.fixture())
        self.assertEqual(21, count)
        self.assertEqual(26, result.count("Missing native source:"))  # includes five source checks replacing legacy apply steps
        for obsolete in ("xui-dependencies", "rebase-panel-protection.py",
                         "rebase-panel-transport.py", "rebase-panel-vless-reverse.py",
                         "sing-vmess-reverse.patch", "xray-runtime-compat.patch"):
            self.assertNotIn(obsolete, result)
        for preserved in ('go mod download "${security_dependencies[@]}"',
                          'python3 "$task_root/tools/verify-hiddify-build.py"',
                          'python3 "$task_root/tools/rebase-panel-masque.py"',
                          "deploy/speed/masque.patch", "go mod tidy",
                          "go build -buildvcs=true"):
            self.assertIn(preserved, result)

    def test_rewrite_rejects_changed_native_mapping_or_dropped_gate(self):
        source = self.fixture()
        for modified in (
            source.replace("vless-reverse.patch", "reverse-v2.patch"),
            source.replace("vision_padding_test.go.in", "vision_padding_v2_test.go.in"),
            source.replace('go mod download "${security_dependencies[@]}"', "true"),
        ):
            with self.subTest(modified=modified != source), self.assertRaises(ValueError):
                self.rewrite_fixture(modified)


if __name__ == "__main__":
    unittest.main()
