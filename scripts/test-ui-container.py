#!/usr/bin/env python3
"""Run the built UI bridge image; fail if Docker or either listener is unavailable.

Build first: docker build -f Docker/Dockerfile.ui -t anthology-ui:local .
Then run: python3 scripts/test-ui-container.py --image anthology-ui:local --verbose
Only disposable containers from the supplied local image are created. Nothing
is pushed, and no API, credentials, production service or browser is required.
"""
import argparse
from html.parser import HTMLParser
import json
import subprocess
import time
import unittest
import urllib.error
import urllib.parse
import urllib.request
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("--image", required=True, help="already-built local UI image")
args, unittest_args = parser.parse_known_args()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, newurl):
        return None


class Assets(HTMLParser):
    def __init__(self):
        super().__init__()
        self.paths = []

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        if tag == "script" and attrs.get("src"):
            self.paths.append(attrs["src"])
        if tag == "link" and attrs.get("rel") == "stylesheet" and attrs.get("href"):
            self.paths.append(attrs["href"])


class UIContainerTests(unittest.TestCase):
    def docker(self, *command, check=True, timeout=60):
        try:
            result = subprocess.run(["docker", *command], capture_output=True, text=True,
                                    timeout=timeout)
        except subprocess.TimeoutExpired:
            self.fail(f"docker {command[0]} timed out after {timeout}s for image {args.image}")
        if check and result.returncode:
            self.fail(f"docker {command[0]} failed: {result.stderr}")
        return result

    def container_name(self):
        name = "anthology-ui-test-" + uuid.uuid4().hex
        self.addCleanup(self.docker, "rm", "--force", name, check=False, timeout=20)
        return name

    def request(self, port, path):
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect)
        request = urllib.request.Request(f"http://127.0.0.1:{port}{path}",
                                         headers={"Host": "anthology.example.test"})
        try:
            response = opener.open(request, timeout=5)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, response.headers, response.read()

    def assert_headers(self, headers):
        self.assertEqual(headers["Server"], "nginx")
        self.assertEqual(headers["X-Content-Type-Options"], "nosniff")
        self.assertEqual(headers["X-Frame-Options"], "DENY")
        self.assertEqual(headers["Referrer-Policy"], "strict-origin-when-cross-origin")
        self.assertEqual(headers["Permissions-Policy"], "camera=(self), microphone=(), geolocation=()")
        csp = headers["Content-Security-Policy"]
        self.assertIn("connect-src 'self' https://api.example.test:8443;", csp)
        self.assertIn("script-src 'self';", csp)
        self.assertIn("frame-ancestors 'none';", csp)
        self.assertNotIn("unsafe-eval", csp)

    def test_both_ports_serve_identical_ui_headers_assets_redirects_and_probes(self):
        metadata = json.loads(self.docker("image", "inspect", args.image).stdout)[0]
        self.assertEqual(set(metadata["Config"]["ExposedPorts"]), {"80/tcp", "8080/tcp"})
        name = self.container_name()
        api_url = "https://api.example.test:8443/v1/api"
        self.docker("run", "--detach", "--name", name,
                    "--publish", "127.0.0.1::80", "--publish", "127.0.0.1::8080",
                    "--env", "NG_APP_API_URL=" + api_url, args.image)
        metadata = json.loads(self.docker("inspect", name).stdout)[0]
        ports = []
        for internal in ["80/tcp", "8080/tcp"]:
            binding = metadata["NetworkSettings"]["Ports"][internal]
            self.assertEqual(len(binding), 1)
            self.assertEqual(binding[0]["HostIp"], "127.0.0.1")
            ports.append(int(binding[0]["HostPort"]))
        for port in ports:
            for attempt in range(100):
                try:
                    if self.request(port, "/ready")[0] == 200:
                        break
                except (OSError, urllib.error.URLError):
                    pass
                time.sleep(.1)
            else:
                self.fail(f"UI listener {port} did not become ready: "
                          + self.docker("logs", name, check=False).stderr)
        self.docker("exec", name, "nginx", "-t")
        paths = [("/", 200), ("/login", 200), ("/items", 200),
                 ("/items/?sort=title", 301), ("/shelves/", 301),
                 ("/missing-route", 404), ("/assets/missing.js", 404),
                 ("/health", 200), ("/ready", 200),
                 ("/assets/runtime-config.js", 200), ("/favicon-32x32.png", 200)]
        for path, expected in paths:
            results = []
            for internal, port in zip([80, 8080], ports):
                with self.subTest(port=internal, path=path):
                    status, headers, body = self.request(port, path)
                    self.assertEqual(status, expected)
                    self.assert_headers(headers)
                    if status == 301:
                        self.assertEqual(headers["Location"],
                                         "/items?sort=title" if path.startswith("/items/") else "/shelves")
                    if path in ["/health", "/ready"]:
                        self.assertEqual(json.loads(body), {"status": "ok"})
                    if path == "/assets/runtime-config.js":
                        self.assertEqual(body.decode(), f'window.NG_APP_API_URL = "{api_url}";\n')
                    results.append((body, headers["Content-Security-Policy"]))
            self.assertEqual(results[0], results[1], f"different responses on bridge ports: {path}")
        assets = Assets()
        assets.feed(self.request(ports[0], "/")[2].decode())
        self.assertIn("/assets/analytics.js", assets.paths)
        self.assertTrue(any(path.endswith(".css") for path in assets.paths))
        self.assertTrue(any("main-" in path and path.endswith(".js") for path in assets.paths))
        for asset in assets.paths:
            if urllib.parse.urlsplit(asset).scheme:
                continue  # The image test never requests external font services.
            path = urllib.parse.urljoin("/", asset)
            bodies = []
            for internal, port in zip([80, 8080], ports):
                with self.subTest(port=internal, asset=path):
                    status, headers, body = self.request(port, path)
                    self.assertEqual(status, 200)
                    self.assert_headers(headers)
                    self.assertNotIn("text/html", headers["Content-Type"])
                    self.assertTrue(body)
                    bodies.append(body)
            self.assertEqual(bodies[0], bodies[1], f"different asset bytes: {path}")

    def test_container_entrypoint_runtime_url_defaults_relative_and_rejection(self):
        for value, expected in [("", "connect-src 'self' http://localhost:8080;"),
                                ("/api", "connect-src 'self';")]:
            with self.subTest(api_url=value):
                result = self.docker("run", "--rm", "--name", self.container_name(),
                                     "--network", "none", "--env", "NG_APP_API_URL=" + value,
                                     args.image, "sh", "-c",
                                     "cat /etc/nginx/snippets/security-headers.conf "
                                     "/usr/share/nginx/html/assets/runtime-config.js")
                self.assertIn(expected, result.stdout)
                if value:
                    self.assertIn('window.NG_APP_API_URL = "/api";', result.stdout)
        for value in ["//evil.example/api", "https://user:password@api.example/api",
                      'https://api.example/";alert(1)', "https://api.example/\nX-Injected: yes"]:
            with self.subTest(invalid_api_url=value):
                result = self.docker("run", "--rm", "--name", self.container_name(),
                                     "--network", "none", "--env", "NG_APP_API_URL=" + value,
                                     args.image, "true", check=False)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Invalid NG_APP_API_URL", result.stderr)


if __name__ == "__main__":
    unittest.main(argv=[__file__] + unittest_args, failfast=True)
