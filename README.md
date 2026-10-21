# Anthology

Anthology is a self-hosted catalogue for books, games, movies, and music. It pairs a Go API with an Angular frontend and includes metadata search, CSV import, cover images, and visual shelf layouts.

## Requirements

- Docker 24+
- PostgreSQL 15+
- A Google Cloud project with:
  - [Google Books API](https://developers.google.com/books/docs/v1/using) enabled and an API key
  - An [OAuth 2.0 Web application client](https://developers.google.com/identity/protocols/oauth2/web-server)

For local OAuth, add this exact authorized redirect URI to the Google client:

```text
http://localhost:8080/api/auth/google/callback
```

Production callbacks must use HTTPS and exactly match `AUTH_GOOGLE_REDIRECT_URL`. Configure at least one allowed email address or domain so the application knows who may sign in.

## Build the images

From the repository root:

```bash
docker build -f Docker/Dockerfile.api -t anthology-api:local .
docker build -f Docker/Dockerfile.ui -t anthology-ui:local .
```

`.dockerignore` excludes Git metadata (including worktree pointer files), saved auth sessions, local configuration and browser/agent output from both build contexts. Run `python3 scripts/test-docker-context.py --verbose` to verify the exclusions using only synthetic files and Docker's actual context export. CI runs this check and builds both production images to ensure their required inputs remain available.

## Configure the API

Create an untracked `anthology.env` file:

```dotenv
# Use production when serving the application over HTTPS.
APP_ENV=development
DATABASE_URL=postgres://anthology:change-me@db:5432/anthology?sslmode=disable
GOOGLE_BOOKS_API_KEY=replace-with-your-api-key
AUTH_GOOGLE_CLIENT_ID=replace-with-your-client-id.apps.googleusercontent.com
AUTH_GOOGLE_CLIENT_SECRET=replace-with-your-client-secret
AUTH_GOOGLE_REDIRECT_URL=http://localhost:8080/api/auth/google/callback
AUTH_GOOGLE_ALLOWED_EMAILS=you@example.com
FRONTEND_URL=http://localhost:4200
ALLOWED_ORIGINS=http://localhost:4200
```

Do not commit this file. `AUTH_GOOGLE_ALLOWED_DOMAINS` can replace or supplement `AUTH_GOOGLE_ALLOWED_EMAILS` with a comma-separated list of Google Workspace domains; a domain entry admits only accounts whose Google `hd` (hosted domain) claim names that domain, not every address with that suffix. The allowlist is rechecked on every request, so removing an address or domain revokes its existing sessions. Expired sessions are purged at startup and daily.

| Setting | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `GOOGLE_BOOKS_API_KEY` | Yes | Google Books metadata lookup |
| `AUTH_GOOGLE_CLIENT_ID` | Yes | Google OAuth client ID |
| `AUTH_GOOGLE_CLIENT_SECRET` | Yes | Google OAuth client secret |
| `AUTH_GOOGLE_ALLOWED_EMAILS` or `AUTH_GOOGLE_ALLOWED_DOMAINS` | Yes | Login allowlist |
| `AUTH_GOOGLE_REDIRECT_URL` | No | OAuth callback; defaults to the local URL above |
| `FRONTEND_URL` | No | Redirect target after authentication |
| `ALLOWED_ORIGINS` | No | Comma-separated browser origins allowed by CORS |
| `APP_ENV` | No | `development` or `production`; defaults to `production` |
| `PORT` | No | API port; defaults to `8080` |
| `LOG_LEVEL` | No | Application log level; defaults to `info` |
| `METRICS_ADDR` | No | Internal Prometheus listener address; defaults to `:9464`, `off` disables it |

The four secret values also support matching `*_FILE` variables. The image defaults to files under `/run/secrets/anthology_*`, making Docker or Kubernetes secret mounts usable without placing credentials in environment variables.

## Run with Docker

Start PostgreSQL and both application containers on a private Docker network:

```bash
docker network create anthology

docker run -d --name db --network anthology \
  -e POSTGRES_DB=anthology \
  -e POSTGRES_USER=anthology \
  -e POSTGRES_PASSWORD=change-me \
  -v anthology-postgres:/var/lib/postgresql/data \
  postgres:17

until docker exec db pg_isready -U anthology -d anthology >/dev/null 2>&1; do sleep 1; done

docker run -d --name anthology-api --network anthology \
  --env-file anthology.env \
  -p 8080:8080 \
  anthology-api:local

docker run -d --name anthology-ui --network anthology \
  -e NG_APP_API_URL=http://localhost:8080/api \
  -p 4200:8080 \
  anthology-ui:local
```

Open <http://localhost:4200>. The API applies embedded Goose migrations automatically during startup. Its public liveness checks, <http://localhost:8080/health> and <http://localhost:8080/api/health>, always return `{"status":"ok"}` while the process is up. Its readiness checks, <http://localhost:8080/ready> and <http://localhost:8080/api/ready>, also ping Postgres (2s timeout) and return 503 `{"db":"error","status":"degraded"}` when the database is unreachable; point orchestrator healthchecks and uptime monitors at `/ready`. The UI container serves static `/health` and `/ready` endpoints because it has no runtime dependencies.

For production, use HTTPS, set `APP_ENV=production`, use a strong database password, restrict the Google API key, and provide secrets through your platform's secret manager.

### UI container runtime

The UI starts as the unprivileged `nginx` user and listens only on container port 8080. Route traffic and healthchecks to 8080; redirects remain relative to the public origin. The unchanged upstream nginx base still advertises port 80 in its image metadata, but the UI does not listen there. Publish 8080 explicitly, as in the Docker example above.

Only `assets/runtime-config.js` and `/etc/nginx/snippets/security-headers.conf` are writable application configuration files. Static bundles, default configuration and their directories remain owned by root. nginx writes its PID and request buffers under `/tmp`, and sends logs to stdout/stderr. Startup needs no root process or added capabilities.

Deployments upgrading from an older port-80 image must first deploy the bridge release that serves both ports, then switch healthchecks and routing to 8080 before using this image. Roll back to that bridge with healthchecks and routing still on 8080.

Validate a built image with `python3 scripts/test-ui-container.py --image anthology-ui:local`. CI builds the real UI image and checks its running PID 1 UID, file permissions, absence of a port-80 listener, and port-8080 headers, routes, assets, probes and runtime API URL validation.

### UI response security

The UI nginx server sends security headers on HTML, assets, redirects, errors and health probes. Its enforced Content Security Policy allows scripts only from the UI origin, including the local analytics bootstrap and `/umami/` proxy. Google Fonts remain allowed. Inline styles are a starter allowance for Angular component styles and style bindings; inline scripts and `eval` are blocked. Production critical CSS inlining is disabled because it generates an inline load handler. Cover images may use HTTP(S) URLs or `data:` thumbnails. Camera permission is limited to this origin for barcode scanning; microphone and geolocation are disabled.

`NG_APP_API_URL` configures both the browser API base URL and the CSP `connect-src` origin at container startup. Use a root-relative path such as `/api` (with your reverse proxy routing it to the API), or an absolute HTTP(S) URL with a DNS/IPv4 host, optional port and path, such as `https://api.example.com:8443/api`. The CSP allows only that origin plus the UI origin, not all HTTPS destinations. Credentials, query strings, fragments, protocol-relative URLs, IPv6 literals and unsafe characters fail startup before configuration is replaced. IPv6 literals are excluded because they are not portable CSP host sources; use a DNS name for an IPv6 service. Without an override, the existing defaults remain: HTTP pages use `http://localhost:8080/api`, HTTPS pages use same-origin `/api`.

All header directives live in `Docker/ui/security-headers.conf.template`. If a location adds any `add_header` of its own, include `/etc/nginx/snippets/security-headers.conf` inside that location too, because nginx otherwise stops inheriting the server headers. Validate startup URL handling with `python3 scripts/test-ui-security.py`; add `--nginx /path/to/nginx` to check nginx syntax and real 200/301/404, asset and probe responses. After a production build, `--web-root web/dist/web/browser` also rejects generated inline scripts and event handlers; pull-request CI runs this check.

### Metrics

The API serves Prometheus metrics at `GET /metrics` on a separate internal listener, `METRICS_ADDR` (default `:9464`), never on the public port, so a reverse proxy routing the public port cannot expose them. Set `METRICS_ADDR=off` (case-insensitive, ignoring surrounding whitespace) to disable the listener. Other values must be `host:port`, with a numeric port from 0 to 65535 and a valid IP address or DNS hostname; an empty host or bracketed IPv6 host (including interface zones) is valid. Invalid address syntax fails configuration loading without DNS lookup; if a valid address cannot be bound, the API logs an error and keeps serving without metrics. Do not publish the metrics port; scrape it over a private network. The listener exposes:

- `http_requests_total` and `http_request_duration_seconds`, labelled by `method`, chi route pattern (`route`, for example `/api/items/{id}`; requests matching no route are `unmatched`), and `status`. `/health`, `/ready`, and `/metrics` are not counted.
- `go_sql_*` connection pool stats for the Postgres pool (`db_name="anthology"`).
- `anthology_oauth_failures_total{provider,reason}`: Google sign-ins that did not end in a session (the only way to log in). Reasons are bounded to `state` (invalid state or missing code), `provider_denied` (provider error or cancellation), `exchange` (token exchange failure), `not_allowed` (unverified or disallowed account), and `internal` (state generation, user persistence, or session failure). Alert only on server-side `reason=~"exchange|internal"`; unknown reasons normalize to `internal` and unknown providers to `unknown`. No email addresses or raw error text become labels.
- `anthology_uploads_total{result}`: CSV import and preview uploads, by `success`, `invalid`, `too_large`, or `error`.
- `anthology_jobs_total{job,result}`: background job runs (`session_cleanup`), by `success` or `error`.
- Go runtime (`go_*`) and process (`process_*`) metrics.

## Development

Copy `local.mk.example` to the ignored `local.mk`, fill in the required values, and run both services:

```bash
make web-install
make local
```

Useful checks:

```bash
make api-test
make web-test
make lint
```

Go checks use golangci-lint v2.11.4, pinned in `.golangci-lint-version`. `make api-lint`, CI and the pre-commit hook share the same version and `.golangci.yml` configuration, including gofmt/goimports checks, security checks, SQL/HTTP resource handling and wrapped-error checks. See [local lint setup](docs/LOCAL_DEVELOPMENT.md#formatting-and-lint) for installation and formatting commands.

Frontend-specific development notes live in [`web/README.md`](web/README.md). Architecture documentation is under [`docs/architecture`](docs/architecture).

## License

Anthology is available under the [MIT License](LICENSE).
