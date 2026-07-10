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

## Configure the API

Create an untracked `anthology.env` file:

```dotenv
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

Do not commit this file. `AUTH_GOOGLE_ALLOWED_DOMAINS` can replace or supplement `AUTH_GOOGLE_ALLOWED_EMAILS` with a comma-separated domain list.

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

docker run -d --name anthology-api --network anthology \
  --env-file anthology.env \
  -p 8080:8080 \
  anthology-api:local

docker run -d --name anthology-ui --network anthology \
  -e NG_APP_API_URL=http://localhost:8080/api \
  -p 4200:80 \
  anthology-ui:local
```

Open <http://localhost:4200>. The API applies embedded Goose migrations automatically during startup. Its public health check is available at <http://localhost:8080/health>.

For production, use HTTPS, set `APP_ENV=production`, use a strong database password, restrict the Google API key, and provide secrets through your platform's secret manager.

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

Frontend-specific development notes live in [`web/README.md`](web/README.md). Architecture documentation is under [`docs/architecture`](docs/architecture).

## License

Anthology is available under the [MIT License](LICENSE).
