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

    def test_retains_legacy_panel_profile(self):
        self.assertEqual("-panel9", prepare.patch_profile_suffix(
            "sawamen-3d4ee155c924-244e333149ec-panel9"))

    def test_rejects_missing_or_arbitrary_profiles(self):
        for version in ("sawamen-abc-def", "sawamen-abc-def-unknown1", "sawamen-abc-def-panel1-extra", "", None):
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
                ]}, "module example.com/core\\ngo 1.26.10\\n",
            )


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


if __name__ == "__main__":
    unittest.main()
