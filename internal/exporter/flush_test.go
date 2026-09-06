package exporter

import (
	"errors"
	"io"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestExportReportsFinalFlushFailure(t *testing.T) {
	if err := NewCSVExporter().Export(failingWriter{}, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected final flush error, got %v", err)
	}
}
