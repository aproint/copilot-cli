import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from ci_changes import classify


class ClassifyChangesTest(unittest.TestCase):
    def test_metadata_only_skips_expensive_checks(self):
        self.assertEqual(
            classify([".github/dependabot.yml", "README.md"]),
            {"full": False, "go": False, "js": False, "license": False},
        )

    def test_go_code_selects_go_and_license(self):
        self.assertEqual(
            classify(["internal/pkg/cli/cli.go"]),
            {"full": False, "go": True, "js": False, "license": True},
        )

    def test_embedded_template_readme_is_go_input(self):
        self.assertTrue(classify(["internal/pkg/template/templates/overrides/cdk/README.md"])["go"])

    def test_js_dependency_change_selects_js_without_license(self):
        self.assertEqual(
            classify(["cf-custom-resources/package-lock.json"]),
            {"full": False, "go": False, "js": True, "license": False},
        )

    def test_mixed_go_and_js_change_selects_both(self):
        checks = classify(["cmd/copilot/main.go", "cf-custom-resources/lib/index.js"])
        self.assertEqual(checks, {"full": False, "go": True, "js": True, "license": True})

    def test_renamed_go_file_keeps_its_old_path_in_diff(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            (root / "internal").mkdir()
            (root / "internal/example.go").write_text("package example\n")
            subprocess.run(["git", "add", "."], cwd=root, check=True)
            subprocess.run(
                [
                    "git", "-c", "user.name=CI", "-c", "user.email=ci@example.com",
                    "-c", "commit.gpgsign=false", "commit", "-qm", "base",
                ],
                cwd=root,
                check=True,
            )
            (root / "internal/example.go").rename(root / "README.md")
            subprocess.run(["git", "add", "-A"], cwd=root, check=True)
            paths = subprocess.check_output(
                ["git", "diff", "--cached", "--no-renames", "--name-only", "-z", "HEAD"], cwd=root
            )
            changed = [os.fsdecode(path) for path in paths.split(b"\0") if path]
            self.assertEqual(changed, ["README.md", "internal/example.go"])
            self.assertTrue(classify(changed)["go"])

    def test_license_script_only_selects_license(self):
        self.assertEqual(
            classify(["scripts/license.sh"]),
            {"full": False, "go": False, "js": False, "license": True},
        )

    def test_unknown_or_ci_control_path_selects_full_suite(self):
        for paths in ([], ["Dockerfile"], [".github/workflows/ci.yml"], ["Makefile"]):
            with self.subTest(paths=paths):
                self.assertEqual(classify(paths), dict.fromkeys(("full", "go", "js", "license"), True))


if __name__ == "__main__":
    unittest.main()
