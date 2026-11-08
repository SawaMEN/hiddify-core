import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare-panel-kernel.py"))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


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


class DependencyOverlayTest(unittest.TestCase):
    def fixture(self, folder, patch):
        panel = folder / "panel"
        (panel / "deploy/hiddify").mkdir(parents=True)
        (panel / "tools").mkdir()
        (panel / "deploy/hiddify/runtime-dependencies.patch").write_text(patch)
        (panel / "tools/build-hiddify-core.sh").write_text(
            'git -C "$task_source/core" apply "$task_root/deploy/hiddify/runtime-dependencies.patch"\n')
        return panel

    def test_rebases_changed_context_and_keeps_newer_existing_version(self):
        patch = "--- a/go.mod\n+++ b/go.mod\n@@ -2,2 +2,4 @@\n require (\n+\texample.com/new v1.0.0 // indirect\n+\texample.com/existing v1.0.0 // indirect\n \texample.com/old v1.0.0 // indirect\n"
        module = "module example.com/core\n\nrequire (\n\texample.com/old v2.0.0 // indirect\n\texample.com/existing v2.0.0 // indirect\n)\n"
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            panel = self.fixture(folder, patch)
            prepare.prepare_runtime_dependencies(panel, module)
            (folder / "go.mod").write_text(module)
            rebased = panel / "deploy/hiddify/runtime-dependencies.patch"
            subprocess.run(["git", "apply", str(rebased)], cwd=folder, check=True)
            actual = (folder / "go.mod").read_text()
            self.assertIn("example.com/new v1.0.0", actual)
            self.assertIn("example.com/existing v2.0.0", actual)
            self.assertNotIn("example.com/existing v1.0.0", actual)

    def test_satisfied_overlay_is_skipped(self):
        patch = "--- a/go.mod\n+++ b/go.mod\n@@ -1 +1,2 @@\n require (\n+\texample.com/existing v1.0.0 // indirect\n"
        with tempfile.TemporaryDirectory() as tmp:
            panel = self.fixture(Path(tmp), patch)
            prepare.prepare_runtime_dependencies(panel, "require (\n\texample.com/existing v2.0.0\n)\n")
            self.assertEqual("", (panel / "deploy/hiddify/runtime-dependencies.patch").read_text())
            self.assertTrue((panel / "tools/build-hiddify-core.sh").read_text().startswith('[ ! -s '))

    def test_rejects_non_additive_overlay(self):
        with tempfile.TemporaryDirectory() as tmp:
            panel = self.fixture(Path(tmp), "--- a/go.mod\n+++ b/go.mod\n-\texample.com/existing v1.0.0\n")
            with self.assertRaises(ValueError):
                prepare.prepare_runtime_dependencies(panel, "require (\n)\n")



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


if __name__ == "__main__":
    unittest.main()
