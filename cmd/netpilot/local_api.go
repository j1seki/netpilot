package main

import (
	"net/http"
	"strings"
)

const localAPIAddress = "127.0.0.1:4545"

var localAPIAllowedOrigins = map[string]struct{}{
	// Wails uses its custom origin on WebKit-based platforms and the localhost
	// origin on Windows WebView2. Do not add arbitrary localhost origins here.
	"wails://wails":      {},
	"http://wails.localhost": {},
}

var localAPIAllowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
}

const localAPIAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"

// newLocalAPIHandler protects the fallback loopback transport from browser-based
// cross-origin requests and DNS rebinding. This API can change routing, TUN state,
// adapter limits, and other host settings, so it must only accept the embedded UI.
func newLocalAPIHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")

		// The listener binds to 127.0.0.1:4545. Requiring that exact Host prevents
		// a hostile site from using a DNS name that resolves to loopback.
		if r.Host != localAPIAddress {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}

		origin := r.Header.Get("Origin")
		if _, ok := localAPIAllowedOrigins[origin]; !ok {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", localAPIAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			method := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
			if _, ok := localAPIAllowedMethods[method]; !ok {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if !localAPIHeadersAllowed(r.Header.Get("Access-Control-Request-Headers")) {
				http.Error(w, "header not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if _, ok := localAPIAllowedMethods[r.Method]; !ok {
			w.Header().Set("Allow", localAPIAllowMethods)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Work on a clone: mutating r.URL.Path directly can affect later middleware
		// or request logging that still holds the original request.
		forwarded := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.Path = strings.TrimPrefix(urlCopy.Path, "/api")
		if urlCopy.Path == "" {
			urlCopy.Path = "/"
		}
		forwarded.URL = &urlCopy
		next.ServeHTTP(w, forwarded)
	})
}

func localAPIHeadersAllowed(requested string) bool {
	for _, header := range strings.Split(requested, ",") {
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if !strings.EqualFold(header, "Content-Type") {
			return false
		}
	}
	return true
}
