package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
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
	authService := auth.NewService(authRepo, 12*time.Hour)

	googleAuth, err := auth.NewGoogleAuthenticator(
		ctx,
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
		cfg.GoogleAllowedDomains,
		cfg.GoogleAllowedEmails,
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

	go func() {
		logger.Info("Anthology API listening", "addr", srv.Addr, "version", version, "revision", revision)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
