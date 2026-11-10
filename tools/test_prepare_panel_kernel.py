import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("prep", Path(__file__).with_name("prepare-panel-kernel.py"))
prep = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prep)

REVIEWED = sorted(prep.REQUIRED_FILES | {
    "hiddify-sing-box/protocol/vless/xui_handshake_test.go",
    "hiddify-sing-box/protocol/vless/xui_preconnect.go",
    "hiddify-sing-box/protocol/vless/xui_reverse_native_test.go",
    "hiddify-sing-box/common/tls/xui_reality_mldsa_handshake_test.go",
    "hiddify-sing-box/protocol/group/xui_balancer_test.go",
    "hiddify-sing-box/h_xray_kcp_test.go",
    "hiddify-sing-box/transport/v2raykcp/h_kcp_test.go",
    "hiddify-sing-box/transport/v2rayxhttp/xui_upload_queue_native_test.go",
    "hiddify-sing-box/third_party/sing-vmess/vless/xui_vision_options.go",
    "hiddify-sing-box/third_party/xray-core/go.mod",
    "hiddify-sing-box/experimental/clashapi/trafficontrol/xui_panel_test.go",
    "hiddify-sing-box/option/xui_panel_http1_test.go",
    "hiddify-sing-box/transport/trojan/xui_stream_test.go",
    "hiddify-sing-box/transport/v2rayhttpupgrade/xui_stream_test.go",
})


def fixture(root):
    (root / "tools").mkdir()
    script = ("python3 tools/verify-hiddify-build.py --upstream src\n"
              "native_source_files\n"
              "go mod tidy\n"
              "go mod download packages\n"
              "go build -buildvcs=true\nsha256sum\n")
    (root / "tools/build-hiddify-core.sh").write_text(script)
    return {
        "panel_build": {"tags": sorted(prep.REQUIRED_TAGS), "native_source_files": REVIEWED},
        "source_checks": [
            {"path": "cmd/cmd_xui_check.go", "contains": 'Use:   "check"'},
            {"path": "hiddify-sing-box/transport/masque/server.go", "contains": 'xuispeed.New("masque:"'},
        ],
    }


class NativeBuildTest(unittest.TestCase):
    def test_native_builder_has_no_panel_patches(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = fixture(root)
            self.assertIs(prep.verify_native_panel(root, lock), lock["panel_build"])

    def test_reject_old_overlay_manifest(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = fixture(root)
            lock["panel_build"]["masque_overlay"] = "old.patch"
            with self.assertRaises(ValueError):
                prep.verify_native_panel(root, lock)

    def test_reject_unverified_speed_broker(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = fixture(root)
            lock["panel_build"]["native_source_files"].remove("hiddify-sing-box/internal/xuispeed/client.go")
            with self.assertRaises(ValueError):
                prep.verify_native_panel(root, lock)

    def test_reject_builder_reintroducing_git_patch(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            lock = fixture(root)
            with (root / "tools/build-hiddify-core.sh").open("a") as script:
                script.write("git apply --check removed.patch\n")
            with self.assertRaises(ValueError):
                prep.verify_native_panel(root, lock)

    def test_validate_abi_suffix(self):
        old = "sawamen-4f1b5e6b9b2b-4fe303071f4f-mask1-ciphers1-net62-sec1-stream1"
        self.assertEqual("-mask1-ciphers1-net62-sec1-stream1", prep.patch_profile_suffix(old))
        with self.assertRaises(ValueError):
            prep.patch_profile_suffix("invalid-profile")

    def test_release_versions_are_reviewed_and_distinct(self):
        core, sing = "a" * 40, "b" * 40
        self.assertEqual("sawamen-" + "a" * 12 + "-" + "b" * 12 + "-mask1",
                         prep.release_tag(core, sing, "stable", "-mask1"))
        self.assertTrue(prep.release_tag(core, sing, "dev", "-mask1").startswith("sawamen-dev-"))
        with self.assertRaises(ValueError):
            prep.release_tag(core, "bad", "stable", "-mask1")


if __name__ == "__main__":
    unittest.main()
