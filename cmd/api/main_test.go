package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeReturnsListenerError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	srv := &http.Server{Addr: listener.Addr().String(), ReadHeaderTimeout: time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	done := make(chan error, 1)
	go func() { done <- serve(context.Background(), srv, logger) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected serve to return the listener error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after ListenAndServe failed")
	}
}

func TestServeShutsDownOnContextCancel(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, logger) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not return after context cancellation")
	}
}
