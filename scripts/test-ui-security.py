#!/usr/bin/env python3
"""Exercise the container entrypoint, optionally with a real nginx executable.

python3 scripts/test-ui-security.py --nginx /path/to/nginx
No Docker daemon, API credentials, or production services are needed.
"""
import argparse
from html.parser import HTMLParser
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("--nginx", help="nginx executable for syntax and HTTP checks")
parser.add_argument("--web-root", type=Path, help="production browser build directory")
args, unittest_args = parser.parse_known_args()


class EntryPointTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="anthology-ui-security-")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.assets = self.directory / "html/assets"
        self.assets.mkdir(parents=True)
        self.headers = self.directory / "security-headers.conf"
        self.default = ROOT / "web/src/assets/runtime-config.js"
        shutil.copy(self.default, self.assets / "runtime-config.default.js")

    def run_entrypoint(self, api_url=None):
        env = dict(os.environ)
        env.pop("NG_APP_API_URL", None)
        env.update(UI_ASSET_DIR=str(self.assets),
                   UI_SECURITY_HEADERS_FILE=str(self.headers),
                   UI_CONFIG_DIR=str(ROOT / "Docker/ui"))
        if api_url is not None:
            env["NG_APP_API_URL"] = api_url
        return subprocess.run(["sh", str(ROOT / "Docker/ui/entrypoint.sh"), "true"],
                              env=env, capture_output=True, text=True)

    def test_default_matches_browser_fallbacks(self):
        self.assertEqual(self.run_entrypoint().returncode, 0)
        self.assertIn("connect-src 'self' http://localhost:8080;", self.headers.read_text())
        self.assertEqual((self.assets / "runtime-config.js").read_bytes(), self.default.read_bytes())

    def test_relative_and_absolute_urls_keep_only_the_api_origin(self):
        for url, origin in [
            ("/api", ""), ("/catalog/v2", ""),
            ("https://api.example.com/api", " https://api.example.com"),
            ("https://API.example.com:443/v2/catalog", " https://api.example.com"),
            ("https://api.example.com:8443/path%20name", " https://api.example.com:8443"),
            ("http://127.0.0.1:8080/api", " http://127.0.0.1:8080"),
            ("http://localhost:80/api", " http://localhost"),
        ]:
            with self.subTest(url=url):
                self.assertEqual(self.run_entrypoint(url).returncode, 0)
                self.assertIn(f"connect-src 'self'{origin};", self.headers.read_text())
                self.assertEqual((self.assets / "runtime-config.js").read_text(),
                                 f'window.NG_APP_API_URL = "{url}";\n')

    def test_invalid_urls_fail_before_overwriting_configuration(self):
        self.assertEqual(self.run_entrypoint("/api").returncode, 0)
        before = self.headers.read_bytes(), (self.assets / "runtime-config.js").read_bytes()
        for url in [
            '//evil.example/api', 'api', 'ftp://api.example/api', 'javascript:alert(1)',
            'https://user:password@api.example/api', 'https://api.example/api?x=1',
            'https://api.example/api#x', 'https://api.example/";alert(1);//',
            "https://api.example/'; script-src *", 'https://api.example/$host',
            'https://api.example/\\path', 'https://api.example/\nX-Injected: yes',
            'https://api.example/\r\n', 'https://api.example/\t',
            'https:///api', 'https://api.example:', 'https://api.example:65536/api',
            'https://api.example:0/api', 'https://api.example:443:80/api',
            'https://*.example/api', 'https://a..example/api', 'https://-api.example/api',
            'https://api-.example/api', 'https://api.example/%zz',
            'http://127.1/api', 'http://0177.0.0.1/api', 'http://256.0.0.1/api',
            'http://2130706433/api', 'http://0x7f000001/api', 'http://[::1]:8080/api',
        ]:
            with self.subTest(url=url):
                result = self.run_entrypoint(url)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Invalid NG_APP_API_URL", result.stderr)
                self.assertNotIn(url, result.stderr)
                self.assertEqual(before, (self.headers.read_bytes(),
                                         (self.assets / "runtime-config.js").read_bytes()))

    def test_restart_without_override_restores_default(self):
        self.assertEqual(self.run_entrypoint("https://api.example/api").returncode, 0)
        self.assertEqual(self.run_entrypoint("").returncode, 0)
        self.assertNotIn("api.example", self.headers.read_text())
        self.assertEqual((self.assets / "runtime-config.js").read_bytes(), self.default.read_bytes())

    def test_baseline_headers_and_strict_scripts(self):
        self.assertEqual(self.run_entrypoint("/api").returncode, 0)
        headers = self.headers.read_text()
        for name in ["X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy",
                     "Permissions-Policy", "Content-Security-Policy"]:
            lines = [line for line in headers.splitlines() if line.startswith(f"add_header {name} ")]
            self.assertEqual(len(lines), 1)
            self.assertTrue(lines[0].endswith(" always;"))
        self.assertIn("script-src 'self';", headers)
        self.assertNotIn("unsafe-eval", headers)
        self.assertIn("camera=(self), microphone=(), geolocation=()", headers)
        self.assertIn("frame-ancestors 'none'", headers)
        # Any future location-specific add_header must re-include the snippet.
        self.assertNotIn("add_header ", (ROOT / "Docker/ui/nginx.conf").read_text())

    @unittest.skipUnless(args.web_root, "pass --web-root to check the production HTML")
    def test_production_html_has_no_inline_script_or_event_handlers(self):
        test = self
        class CheckHTML(HTMLParser):
            def handle_starttag(self, tag, attributes):
                attrs = dict(attributes)
                test.assertFalse(any(key.startswith("on") for key in attrs),
                                 f"inline event handler on {tag}")
                if tag == "script":
                    test.assertTrue(attrs.get("src"), "inline script in production HTML")
        html = (args.web_root / "index.html").read_text()
        CheckHTML().feed(html)
        self.assertIn('/assets/analytics.js', html)
        self.assertTrue((args.web_root / "assets/analytics.js").is_file())

    @unittest.skipUnless(args.nginx, "pass --nginx to include real nginx HTTP checks")
    def test_nginx_syntax_and_headers_on_success_redirect_error_assets_and_probes(self):
        self.assertEqual(self.run_entrypoint("https://api.example:8443/api").returncode, 0)
        html = self.assets.parent
        (html / "index.html").write_text("<!doctype html><title>UI security check</title>")
        (self.assets / "example.css").write_text("body { color: black; }")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        server = (ROOT / "Docker/ui/nginx.conf").read_text()
        server = server.replace("listen 80;", f"listen 127.0.0.1:{port};")
        server = server.replace("    listen [::]:80;\n", "")
        server = server.replace("/usr/share/nginx/html", str(html))
        server = server.replace("/etc/nginx/snippets/security-headers.conf", str(self.headers))
        config = self.directory / "nginx.conf"
        config.write_text(f'pid "{self.directory}/nginx.pid";\nerror_log stderr;\n'
                          f'events {{}}\nhttp {{ access_log off; {server} }}\n')
        command = [args.nginx, "-p", str(self.directory), "-c", str(config)]
        subprocess.run(command + ["-t"], check=True, capture_output=True)
        log = (self.directory / "nginx.log").open("w")
        self.addCleanup(log.close)
        process = subprocess.Popen(command + ["-g", "daemon off;"], stdout=log, stderr=log)
        def stop():
            process.terminate()
            process.wait(timeout=10)
        self.addCleanup(stop)
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, request, fp, code, msg, headers, newurl):
                return None
        opener = urllib.request.build_opener(NoRedirect)
        for attempt in range(100):
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=.1):
                    break
            except OSError:
                time.sleep(.02)
        for path, status in [("/", 200), ("/login", 200), ("/items/", 301),
                             ("/unknown-path", 404), ("/assets/missing.css", 404),
                             ("/assets/example.css", 200), ("/health", 200), ("/ready", 200)]:
            with self.subTest(path=path):
                try:
                    response = opener.open(f"http://127.0.0.1:{port}{path}")
                except urllib.error.HTTPError as error:
                    response = error
                with response:
                    self.assertEqual(response.status, status)
                    self.assertEqual(response.headers["Server"], "nginx")
                    self.assertEqual(response.headers["X-Content-Type-Options"], "nosniff")
                    self.assertEqual(response.headers["X-Frame-Options"], "DENY")
                    self.assertEqual(response.headers["Referrer-Policy"], "strict-origin-when-cross-origin")
                    self.assertEqual(response.headers["Permissions-Policy"], "camera=(self), microphone=(), geolocation=()")
                    self.assertIn("connect-src 'self' https://api.example:8443;",
                                  response.headers["Content-Security-Policy"])


if __name__ == "__main__":
    unittest.main(argv=[__file__] + unittest_args)
