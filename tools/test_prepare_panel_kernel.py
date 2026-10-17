import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare-panel-kernel.py"))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


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
