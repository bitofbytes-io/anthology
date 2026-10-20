#!/bin/sh
set -eu

# Overrides also allow exercising the production entrypoint without root.
CONFIG_DIR="${UI_ASSET_DIR:-/usr/share/nginx/html/assets}"
CONFIG_FILE="$CONFIG_DIR/runtime-config.js"
DEFAULT_FILE="$CONFIG_DIR/runtime-config.default.js"
HEADERS_FILE="${UI_SECURITY_HEADERS_FILE:-/etc/nginx/snippets/security-headers.conf}"
SUPPORT_DIR="${UI_CONFIG_DIR:-/etc/anthology-ui}"

if [ -n "${NG_APP_API_URL:-}" ]; then
    # The validator excludes JavaScript/nginx metacharacters. Fail before
    # replacing either file if the value is unsafe or not a supported URL.
    origin=$(LC_ALL=C awk -f "$SUPPORT_DIR/runtime-api-origin.awk")
    api_origins=${origin:+" $origin"}
else
    # Match environment.ts: HTTP uses the baked localhost API fallback;
    # HTTPS uses the same-origin /api fallback (mixed content is blocked).
    api_origins=" http://localhost:8080"
fi

mkdir -p "$CONFIG_DIR" "$(dirname "$HEADERS_FILE")"
sed "s|__API_ORIGINS__|$api_origins|g" "$SUPPORT_DIR/security-headers.conf.template" > "$HEADERS_FILE"

if [ -n "${NG_APP_API_URL:-}" ]; then
    printf 'window.NG_APP_API_URL = "%s";\n' "$NG_APP_API_URL" > "$CONFIG_FILE"
elif [ -f "$DEFAULT_FILE" ]; then
    cp "$DEFAULT_FILE" "$CONFIG_FILE"
else
    printf 'window.NG_APP_API_URL = undefined;\n' > "$CONFIG_FILE"
fi

exec "$@"
