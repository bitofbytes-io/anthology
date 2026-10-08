package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"anthology/internal/importer"
)

func TestRequestTimeoutMiddlewareGivesCSVImportLongerBudget(t *testing.T) {
	cases := []struct {
		method, path string
		want         time.Duration
	}{
		{http.MethodPost, csvImportPath, csvImportTimeout},
		{http.MethodPost, csvImportPreviewPath, csvImportTimeout},
		{http.MethodPost, csvImportCommitPath, csvImportTimeout},
		{http.MethodGet, csvImportPreviewPath, requestTimeout},
		{http.MethodGet, csvImportPath, requestTimeout},
		{http.MethodPost, "/api/items", requestTimeout},
		{http.MethodGet, "/api/shelves", requestTimeout},
	}
	for _, tc := range cases {
		var remaining time.Duration
		handler := newRequestTimeoutMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			if !ok {
				t.Fatalf("%s %s: expected a request deadline", tc.method, tc.path)
			}
			remaining = time.Until(deadline)
		}))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, tc.path, nil))
		if remaining > tc.want || remaining < tc.want-5*time.Second {
			t.Fatalf("%s %s: deadline in %s, want about %s", tc.method, tc.path, remaining, tc.want)
		}
	}
	if importer.LookupBudget >= csvImportTimeout {
		t.Fatalf("lookup budget %s must fit inside the import request timeout %s", importer.LookupBudget, csvImportTimeout)
	}
	if csvImportWriteDeadline <= csvImportTimeout {
		t.Fatalf("import write deadline %s must outlast the import request timeout %s", csvImportWriteDeadline, csvImportTimeout)
	}
}
