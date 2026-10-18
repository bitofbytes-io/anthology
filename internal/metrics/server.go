package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server is the internal metrics listener. A nil *Server (disabled, or the
// address could not be bound) is valid and Shutdown is a no-op.
type Server struct {
	srv  *http.Server
	addr net.Addr
}

// NewMux returns a mux serving handler at GET /metrics and nothing else.
func NewMux(handler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)
	return mux
}

// Start serves handler at GET /metrics on addr, separate from the public
// listener. An empty addr disables the listener. A bind failure is logged
// and returns nil rather than stopping the API.
func Start(addr string, handler http.Handler, logger *slog.Logger) *Server {
	if addr == "" {
		logger.Info("metrics listener disabled")
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("metrics listener failed to bind; continuing without metrics", "addr", addr, "error", err)
		return nil
	}

	srv := &http.Server{
		Handler:           NewMux(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics listener stopped", "error", err)
		}
	}()
	logger.Info("metrics listening", "addr", ln.Addr().String())
	return &Server{srv: srv, addr: ln.Addr()}
}

// Addr returns the bound address, or nil when s is nil.
func (s *Server) Addr() net.Addr {
	if s == nil {
		return nil
	}
	return s.addr
}

// Shutdown gracefully stops the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
