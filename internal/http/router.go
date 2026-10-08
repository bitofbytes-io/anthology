package http

import (
	"net/http"
	"time"

	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"anthology/internal/auth"
	"anthology/internal/catalog"
	"anthology/internal/config"
	"anthology/internal/importer"
	"anthology/internal/items"
	"anthology/internal/shelves"
)

// requestTimeout bounds how long any handler may run before its context is
// cancelled and chi responds with 504 Gateway Timeout.
const requestTimeout = 60 * time.Second

// CSV import routes, which get csvImportTimeout instead of requestTimeout:
// the single-request import, the read-only preview, and the reviewed commit.
const (
	csvImportPath        = "/api/items/import"
	csvImportPreviewPath = "/api/items/import/preview"
	csvImportCommitPath  = "/api/items/import/commit"
)

// csvImportTimeout bounds a CSV import request. It covers the upload (bounded by
// the server's 15s ReadTimeout), the importer's 30s lookup budget
// (importer.LookupBudget, spent by imports and previews), and up to 1000 row
// inserts (imports and commits), with headroom. The import write deadline
// (csvImportWriteDeadline) is longer still, so the summary can be written even
// when the import is cut short.
const csvImportTimeout = 75 * time.Second

func isCSVImportPath(path string) bool {
	switch path {
	case csvImportPath, csvImportPreviewPath, csvImportCommitPath:
		return true
	default:
		return false
	}
}

// newRequestTimeoutMiddleware applies chi's Timeout middleware, giving CSV
// imports csvImportTimeout and every other request requestTimeout.
func newRequestTimeoutMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		standard := middleware.Timeout(requestTimeout)(next)
		csvImport := middleware.Timeout(csvImportTimeout)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && isCSVImportPath(r.URL.Path) {
				csvImport.ServeHTTP(w, r)
				return
			}
			standard.ServeHTTP(w, r)
		})
	}
}

// NewRouter wires application routes and middleware using chi.
func NewRouter(cfg config.Config, svc *items.Service, catalogSvc *catalog.Service, shelfSvc *shelves.Service, authService *auth.Service, googleAuth *auth.GoogleAuthenticator, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(newRequestTimeoutMiddleware())
	r.Use(newSecurityHeadersMiddleware(cfg.Environment))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(newSameOriginMiddleware(cfg.AllowedOrigins))
	r.Use(newSlogMiddleware(logger))

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
		})
	}
	r.Get("/health", healthHandler)

	sessionHandler := NewSessionHandler(authService, cfg.Environment, logger)
	bulkImporter := importer.NewCSVImporter(svc, catalogSvc)
	handler := NewItemHandler(svc, catalogSvc, bulkImporter, logger)
	catalogHandler := NewCatalogHandler(catalogSvc, logger)
	shelfHandler := NewShelfHandler(shelfSvc, logger)
	seriesHandler := NewSeriesHandler(svc, logger)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler)

		// OAuth routes (unauthenticated)
		if googleAuth != nil {
			oauthHandler := NewOAuthHandler(googleAuth, authService, cfg.FrontendURL, cfg.Environment, logger)
			r.Route("/auth", func(r chi.Router) {
				r.Get("/google", oauthHandler.InitiateGoogle)
				r.Get("/google/callback", oauthHandler.CallbackGoogle)
			})
		}

		// Session routes (unauthenticated - for checking status and logout)
		r.Route("/session", func(r chi.Router) {
			r.Get("/", sessionHandler.Status)
			r.Delete("/", sessionHandler.Logout)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(newAuthMiddleware(authService, logger))

			// User info endpoint
			r.Get("/session/user", sessionHandler.CurrentUser)

			r.Route("/items", func(r chi.Router) {
				r.Get("/", handler.List)
				r.Get("/histogram", handler.Histogram)
				r.Get("/duplicates", handler.Duplicates)
				r.Get("/export", handler.ExportCSV)
				r.Post("/", handler.Create)
				r.Post("/import", handler.ImportCSV)
				r.Post("/import/preview", handler.PreviewCSVImport)
				r.Post("/import/commit", handler.CommitCSVImport)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", handler.Get)
					r.Put("/", handler.Update)
					r.Delete("/", handler.Delete)
					r.Post("/resync", handler.Resync)
				})
			})
			r.Route("/series", func(r chi.Router) {
				r.Get("/", seriesHandler.List)
				r.Get("/detail", seriesHandler.Get)
				r.Put("/detail", seriesHandler.Update)
				r.Delete("/detail", seriesHandler.Delete)
			})
			r.Route("/shelves", func(r chi.Router) {
				r.Get("/", shelfHandler.List)
				r.Post("/", shelfHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", shelfHandler.Get)
					r.Put("/layout", shelfHandler.UpdateLayout)
					r.Route("/slots/{slotId}", func(r chi.Router) {
						r.Post("/scan", shelfHandler.ScanAndAssign)
						r.Route("/items", func(r chi.Router) {
							r.Post("/", shelfHandler.AssignItem)
							r.Delete("/{itemId}", shelfHandler.RemoveItem)
						})
					})
				})
			})
			r.Route("/catalog", func(r chi.Router) {
				r.Get("/lookup", catalogHandler.Lookup)
			})
		})
	})

	r.NotFound(http.NotFoundHandler().ServeHTTP)

	return r
}
