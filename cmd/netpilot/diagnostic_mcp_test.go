package main

import (
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiagnosticMCPRejectsUntrustedRequests(t *testing.T) {
	const host = "127.0.0.1:4546"
	const token = "test-token"
	svc := &NetPilotService{}
	handler := svc.diagnosticMCPHandler(host, token)
	for _, tc := range []struct {
		path, address, origin, auth string
		want                        int
	}{
		{"/mcp", host, "", "", http.StatusUnauthorized},
		{"/mcp", host, "", "Bearer invalid", http.StatusUnauthorized},
		{"/mcp", host, "http://other.example", "Bearer " + token, http.StatusForbidden},
		{"/mcp", "other.example:4546", "", "Bearer " + token, http.StatusForbidden},
		{"/api/netmerge/toggle", host, "", "Bearer " + token, http.StatusNotFound},
		{"/mcp", host, "", "Bearer " + token, http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://"+tc.address+tc.path,
			strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Authorization", tc.auth)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != tc.want {
			t.Fatalf("%+v yielded %d: %s", tc, recorder.Code, recorder.Body)
		}
		if tc.want == http.StatusOK {
			var reply struct {
				Result struct {
					Tools []struct{ Name string }
				}
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{"status": true, "adapters": true, "process_sockets": true, "run_diagnostics": true}
			for _, tool := range reply.Result.Tools {
				if !want[tool.Name] {
					t.Errorf("unexpected MCP tool: %s", tool.Name)
				}
				delete(want, tool.Name)
			}
			if len(want) != 0 {
				t.Errorf("missing MCP tools: %v", want)
			}
		}
	}
}

func TestDiagnosticLANCertificatePinsSelectedAddress(t *testing.T) {
	cert, pem, err := diagnosticTLS(net.ParseIP("192.0.2.12"))
	if err != nil {
		t.Fatal(err)
	}
	root := x509.NewCertPool()
	if !root.AppendCertsFromPEM([]byte(pem)) {
		t.Fatal("invalid certificate")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: root, DNSName: "192.0.2.12"}); err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: root, DNSName: "192.0.2.13"}); err == nil {
		t.Fatal("certificate should only authenticate the selected address")
	}
}
