#!/usr/bin/env python3
"""Check Docker's actual context filtering using synthetic files only.

Run: python3 scripts/test-docker-context.py --verbose
Requires Docker with BuildKit/local export. No workspace credentials or sessions
are copied, and no application containers or production services are started.
"""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

# Representative inputs for both production Dockerfiles. Real API/UI builds
# in CI additionally check the complete source tree and dependency inputs.
REQUIRED_FILES = [
    "go.mod", "go.sum", "cmd/api/main.go", "internal/auth/google.go",
    "migrations/embed.go", "migrations/0001_baseline.sql",
    "web/package.json", "web/package-lock.json", "web/angular.json",
    "web/tsconfig.json", "web/tsconfig.app.json",
    "web/src/main.ts", "web/src/index.html", "web/src/styles.scss",
    "web/src/app/app.ts", "web/src/assets/runtime-config.js",
    "web/src/assets/analytics.js",
    "Docker/Dockerfile.api", "Docker/Dockerfile.ui",
    "Docker/ui/entrypoint.sh", "Docker/ui/nginx.conf", "Docker/ui/nginx-main.conf",
    "Docker/ui/runtime-api-origin.awk", "Docker/ui/security-headers.conf.template",
]

PRIVATE_FILES = [
    ".auth/synthetic-session.json", "local.mk", ".local/synthetic-config",
    ".worktree.json", ".playwright-cli/synthetic-browser-state",
    "output/playwright/synthetic-session.json",
]

EXISTING_EXCLUSIONS = [
    "bin/synthetic-api", "build.log", "coverage.out", "anthology.env",
    "web/node_modules/synthetic-package/index.js", "web/dist/index.html",
    "web/.angular/synthetic-cache", "web/.sass-cache/synthetic-cache",
    "web/.turbo/synthetic-cache", "web/.DS_Store",
    ".vscode/settings.json", ".idea/workspace.xml", ".DS_Store",
]


class DockerContextTests(unittest.TestCase):
    def assert_context(self, git_is_file):
        with tempfile.TemporaryDirectory(prefix="anthology-docker-context-") as temp:
            directory = Path(temp)
            context = directory / "input"
            context.mkdir()
            # Only the ignore rules come from the repository. Every other
            # input below is a synthetic sentinel, never a workspace copy.
            (context / ".dockerignore").write_bytes((ROOT / ".dockerignore").read_bytes())
            excluded = PRIVATE_FILES + EXISTING_EXCLUSIONS
            if git_is_file:
                excluded += [".git", ".gemini-clipboard"]
            else:
                excluded += [".git/config", ".git/objects/synthetic-object",
                             ".gemini-clipboard/synthetic-clipboard"]
            for name in REQUIRED_FILES + excluded:
                path = context / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("synthetic sentinel: " + name + "\n")
            if git_is_file:
                (context / ".git").write_text("gitdir: /synthetic/nonexistent/worktree\n")
            exported = directory / "export"
            try:
                result = subprocess.run(
                    ["docker", "build", "--progress=plain", "--file", "-",
                     "--output", f"type=local,dest={exported}", str(context)],
                    input="FROM scratch\nCOPY . /context/\n",
                    text=True, capture_output=True, timeout=60,
                    env={**os.environ, "DOCKER_BUILDKIT": "1", "BUILDX_GIT_INFO": "false"},
                )
            except subprocess.TimeoutExpired:
                self.fail("Docker context export timed out after 60 seconds")
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            visible = exported / "context"
            for name in REQUIRED_FILES:
                with self.subTest(required=name):
                    self.assertEqual((visible / name).read_text(),
                                     "synthetic sentinel: " + name + "\n")
            for name in excluded:
                with self.subTest(excluded=name):
                    self.assertFalse((visible / name).exists(), f"private/ignored path exported: {name}")
            for name in [".git", ".auth", ".local", ".playwright-cli", "output", ".gemini-clipboard"]:
                with self.subTest(excluded_root=name):
                    self.assertFalse((visible / name).exists(), f"private root exported: {name}")

    def test_context_excludes_git_directory_and_private_artifacts(self):
        self.assert_context(git_is_file=False)

    def test_context_excludes_git_worktree_file_and_private_artifacts(self):
        self.assert_context(git_is_file=True)


if __name__ == "__main__":
    unittest.main()
