# Local Development Setup

This guide walks through setting up Anthology for local development with full OAuth authentication.

## Prerequisites

- PostgreSQL database running locally
- Go 1.26+
- Node.js 24+ (for Angular frontend)
- Google Cloud account

## Formatting and lint

Install the [golangci-lint v2.11.4 release](https://github.com/golangci/golangci-lint/releases/tag/v2.11.4) for your platform and put `golangci-lint` on `PATH`. The required version is recorded in `.golangci-lint-version`; `make api-lint` rejects other versions, and CI installs that same pin.

```bash
# Check the API without installing frontend dependencies.
make api-lint

# Apply the configured gofmt and goimports formatting, then check again.
golangci-lint fmt
make api-lint

# Check both Go and the installed Angular workspace.
make lint

# Enable the repository's pre-commit hook.
git config core.hooksPath githooks
```

The hook runs `make api-lint` before Angular lint. `.golangci.yml` explicitly enables errcheck, govet, staticcheck, gosec, bodyclose, sqlclosecheck, rowserrcheck and errorlint, plus the existing ineffassign and unused checks. Formatting is checked without rewriting files during lint. Keep any false-positive suppression at the specific line with a reason; do not exclude whole files or test suites.

## Database Setup

Create the local database:

```bash
createdb anthology
```

Or with a specific user:

```sql
CREATE USER anthology WITH PASSWORD 'anthology';
CREATE DATABASE anthology OWNER anthology;
```

The default connection string is:
```
postgres://anthology:anthology@localhost:5432/anthology?sslmode=disable
```

## Google OAuth Setup

### Step 1: Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Note your project ID for reference

### Step 2: Configure OAuth Consent Screen

1. Navigate to **APIs & Services** > **OAuth consent screen**
2. Select **External** user type (unless you have a Google Workspace organization)
3. Fill in the required fields:
   - App name: `Anthology (Development)`
   - User support email: Your email
   - Developer contact: Your email
4. Click **Save and Continue**
5. Skip scopes (defaults are sufficient)
6. Add your email as a test user
7. Complete the setup

### Step 3: Create OAuth 2.0 Credentials

1. Navigate to **APIs & Services** > **Credentials**
2. Click **Create Credentials** > **OAuth 2.0 Client IDs**
3. Select **Web application** as the application type
4. Name it `Anthology Local Development`
5. Configure the following:

**Authorized JavaScript origins:**
```
http://localhost:4200
```

**Authorized redirect URIs:**
```
http://localhost:8080/api/auth/google/callback
```

6. Click **Create**
7. Copy the **Client ID** and **Client Secret**

### Step 4: Configure local.mk

Copy the example configuration:

```bash
cp local.mk.example local.mk
```

Edit `local.mk` with your credentials:

```makefile
DATABASE_URL = postgres://anthology:anthology@localhost:5432/anthology?sslmode=disable
GOOGLE_BOOKS_API_KEY = your-google-books-api-key

AUTH_GOOGLE_CLIENT_ID = your-client-id.apps.googleusercontent.com
AUTH_GOOGLE_CLIENT_SECRET = your-client-secret
AUTH_GOOGLE_ALLOWED_EMAILS = you@gmail.com

APP_ENV = development
```

**Important:**
- `local.mk` is gitignored and should never be committed
- `APP_ENV=development` is required for local development (enables HTTP cookies). If omitted, it defaults to `production` which requires HTTPS.

## Running the Application

Start both the API and Angular frontend:

```bash
make local
```

Or run them separately:

```bash
# Terminal 1 - API
make api-run

# Terminal 2 - Frontend
make web-start
```

The application will be available at:
- Frontend: http://localhost:4200
- API: http://localhost:8080

## Saved auth state for local agent verification

If you want local agents to verify UI behavior without adding auth bypasses, capture a Playwright `storageState` after you log in manually.

1. Start the app (`make local`) and log in normally in the browser.
2. Run the capture tool:

```bash
node scripts/auth-capture.js
```

This opens a headed browser, waits for you to confirm login (press Enter), and saves:

- `./.auth/<appName>.json`

Refresh it anytime by re-running the same command.

## OAuth Configuration for Production (anthology.bitofbytes.io)

**Authorized JavaScript origins:**
```
https://anthology.bitofbytes.io
```

**Authorized redirect URIs:**
```
https://anthology.bitofbytes.io/api/auth/google/callback
```

**Tip:** You can add multiple origins and redirect URIs to a single OAuth client, or create separate clients per environment for better isolation.

## Troubleshooting

### "redirect_uri_mismatch" error

The redirect URI in your OAuth client doesn't match the one the application is using. Verify:
1. `AUTH_GOOGLE_REDIRECT_URL` in local.mk matches exactly what's in Google Console
2. Default is `http://localhost:8080/api/auth/google/callback`
3. Check for trailing slashes or protocol mismatches

### "access_denied" error

Your email is not in the allowlist. Check:
1. `AUTH_GOOGLE_ALLOWED_EMAILS` includes your email address
2. Or `AUTH_GOOGLE_ALLOWED_DOMAINS` includes your email domain, and your account is managed by that Google Workspace domain (domain entries check Google's `hd` claim, so a personal Google account with an address on that domain is rejected)

### Database connection errors

1. Verify PostgreSQL is running: `pg_isready`
2. Check the connection string in `DATABASE_URL`
3. Ensure the database exists: `psql -l | grep anthology`

### OAuth consent screen in "Testing" mode

While in testing mode, only users added as test users can authenticate. Either:
1. Add your email as a test user in Google Console
2. Or publish the OAuth consent screen (for production use)
