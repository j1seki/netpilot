package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func apiRequest(method, origin, host string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:4545/api/netmerge/status?details=1", nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestLocalAPIAllowsOnlyEmbeddedWailsOrigins(t *testing.T) {
	for _, origin := range []string{"wails://wails", "http://wails.localhost"} {
		t.Run(origin, func(t *testing.T) {
			called := false
			handler := newLocalAPIHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != "/netmerge/status" || r.URL.Query().Get("details") != "1" {
					t.Errorf("forwarded URL = %q, want /netmerge/status?details=1", r.URL.String())
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, apiRequest(http.MethodGet, origin, localAPIAddress))

			if !called || w.Code != http.StatusAccepted {
				t.Fatalf("handler called=%v status=%d, want called and 202", called, w.Code)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("allow-origin = %q, want %q", got, origin)
			}
		})
	}
}

func TestLocalAPIRejectsUntrustedOrMissingOrigin(t *testing.T) {
	for _, origin := range []string{"https://attacker.example", ""} {
		t.Run(origin, func(t *testing.T) {
			called := false
			handler := newLocalAPIHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, apiRequest(http.MethodGet, origin, localAPIAddress))

			if called || w.Code != http.StatusForbidden {
				t.Fatalf("handler called=%v status=%d, want blocked with 403", called, w.Code)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("denied origin received CORS permission %q", got)
			}
		})
	}
}

func TestLocalAPIRejectsDNSRebindingHost(t *testing.T) {
	called := false
	handler := newLocalAPIHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, apiRequest(http.MethodPost, "wails://wails", "attacker.example:4545"))

	if called || w.Code != http.StatusForbidden {
		t.Fatalf("handler called=%v status=%d, want blocked with 403", called, w.Code)
	}
}

func TestLocalAPIPreflightValidatesMethodAndHeaders(t *testing.T) {
	t.Run("allowed JSON POST", func(t *testing.T) {
		handler := newLocalAPIHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("preflight must not reach the service")
		}))
		r := apiRequest(http.MethodOptions, "wails://wails", localAPIAddress)
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "content-type")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
	})

	t.Run("reject unlisted header", func(t *testing.T) {
		handler := newLocalAPIHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("rejected preflight must not reach the service")
		}))
		r := apiRequest(http.MethodOptions, "wails://wails", localAPIAddress)
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "content-type, x-private-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("reject unlisted method", func(t *testing.T) {
		handler := newLocalAPIHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("rejected preflight must not reach the service")
		}))
		r := apiRequest(http.MethodOptions, "wails://wails", localAPIAddress)
		r.Header.Set("Access-Control-Request-Method", "TRACE")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", w.Code)
		}
	})
}
