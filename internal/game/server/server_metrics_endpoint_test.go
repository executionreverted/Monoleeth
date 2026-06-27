package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/observability"
)

func TestMetricsEndpointExposesCommandCount(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	defer gameServer.Shutdown(context.Background())

	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-session-snapshot-metrics","op":"session.snapshot","payload":{},"client_seq":1,"v":1}`)
	response := readResponseSkippingEvents(t, conn)
	if !response.OK {
		t.Fatalf("session snapshot response = %+v, want success", response)
	}

	resp, err := http.Get(httpServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, observability.PrometheusContentType) {
		t.Fatalf("/metrics content-type = %q, want %q", got, observability.PrometheusContentType)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics: %v", err)
	}
	if !strings.Contains(string(body), `commands_per_sec{op="session.snapshot"} 1`) {
		t.Fatalf("/metrics body missing command count:\n%s", body)
	}
}

func TestMetricsEndpointRequiresBearerTokenWhenConfigured(t *testing.T) {
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		MetricsToken:      "test-metrics-token",
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	defer httpServer.Close()
	defer gameServer.Shutdown(context.Background())

	for _, tc := range []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong", authorization: "Bearer wrong-token", wantStatus: http.StatusUnauthorized},
		{name: "plain", authorization: "test-metrics-token", wantStatus: http.StatusUnauthorized},
		{name: "correct", authorization: "Bearer test-metrics-token", wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/metrics", nil)
			if err != nil {
				t.Fatalf("new metrics request: %v", err)
			}
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET /metrics: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("/metrics status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}
