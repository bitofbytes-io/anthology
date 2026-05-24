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

// NewRouter wires application routes and middleware using chi.
func NewRouter(cfg config.Config, svc *items.Service, catalogSvc *catalog.Service, shelfSvc *shelves.Service, authService *auth.Service, googleAuth *auth.GoogleAuthenticator, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
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

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
		})
	})

	sessionHandler := NewSessionHandler(authService, cfg.Environment, logger)
	bulkImporter := importer.NewCSVImporter(svc, catalogSvc)
	handler := NewItemHandler(svc, catalogSvc, bulkImporter, logger)
	catalogHandler := NewCatalogHandler(catalogSvc, logger)
	shelfHandler := NewShelfHandler(shelfSvc, logger)
	seriesHandler := NewSeriesHandler(svc, logger)

	r.Route("/api", func(r chi.Router) {
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
