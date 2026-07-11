# Agent Guidance

- Treat `web/src/assets/runtime-config.js` as runtime configuration: the UI container rewrites it from `NG_APP_API_URL` without rebuilding the Angular bundle.
- Keep API and UI deployment concerns separate; the project intentionally publishes two images.
- Do not add authentication bypass routes. For browser verification, reuse manually captured Playwright state under the ignored `.auth/` directory and never commit it.
- Update source files rather than generated Angular build output under `web/dist/`.
- Use `make local` to run the API and Angular dev server together after configuring ignored local settings.
- Use `make api-test`, `make web-test`, and `make lint` for validation; UI changes should also receive browser verification when the application is available.
- Capture browser auth state with `make auth-capture` only into the ignored `.auth/` directory.
- Preserve the startup migration path: the API applies embedded Goose migrations before serving requests.
