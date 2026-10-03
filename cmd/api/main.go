package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"anthology/internal/auth"
	"anthology/internal/catalog"
	"anthology/internal/config"
	transporthttp "anthology/internal/http"
	"anthology/internal/items"
	"anthology/internal/platform/database"
	"anthology/internal/platform/logging"
	"anthology/internal/platform/migrate"
	"anthology/internal/shelves"
)

var (
	version  = "dev"
	revision = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := logging.New(cfg.LogLevel)

	// Connect to Postgres
	db, err := database.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database", "error", err)
		}
	}()

	// Apply migrations
	if err := migrate.Apply(ctx, db, logger); err != nil {
		logger.Error("failed to apply migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to postgres", "version", version, "revision", revision)

	// Initialize repositories
	itemRepo := items.NewPostgresRepository(db)
	shelfRepo := shelves.NewPostgresRepository(db)

	// Initialize auth (always required)
	authRepo := auth.NewPostgresRepository(db)
	allowlist := auth.NewAllowlist(cfg.GoogleAllowedDomains, cfg.GoogleAllowedEmails)
	authService := auth.NewService(authRepo, 12*time.Hour, allowlist.AllowsEmail)

	googleAuth, err := auth.NewGoogleAuthenticator(
		ctx,
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
		allowlist,
	)
	if err != nil {
		logger.Error("failed to initialize Google OAuth", "error", err)
		os.Exit(1)
	}
	logger.Info("Google OAuth enabled", "redirect_url", cfg.GoogleRedirectURL)

	svc := items.NewService(itemRepo)
	lookupClient := &http.Client{Timeout: 12 * time.Second}
	catalogSvc := catalog.NewService(lookupClient, catalog.WithGoogleBooksAPIKey(cfg.GoogleBooksAPIKey))
	shelfSvc := shelves.NewService(shelfRepo, itemRepo, catalogSvc, svc)
	router := transporthttp.NewRouter(cfg, svc, catalogSvc, shelfSvc, authService, googleAuth, logger)

	srv := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
	}

	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	var cleanup sync.WaitGroup
	cleanup.Add(1)
	go func() {
		defer cleanup.Done()
		authService.ScheduleSessionCleanup(cleanupCtx, auth.SessionCleanupInterval, logger)
	}()

	logger.Info("Anthology API listening", "addr", srv.Addr, "version", version, "revision", revision)
	serveErr := serve(ctx, srv, logger)
	stopCleanup()
	cleanup.Wait()
	if serveErr != nil {
		logger.Error("http server error", "error", serveErr)
		os.Exit(1)
	}
}

// serve runs srv until it fails or ctx is cancelled. A listener failure (for
// example, the port already being in use) is returned so the process can exit
// non-zero instead of idling until a shutdown signal arrives.
func serve(ctx context.Context, srv *http.Server, logger *slog.Logger) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	return nil
}
