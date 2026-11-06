package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A zero interval (RELAYD_INTERVAL=0) must be rejected, not crash the ticker.
func TestRunRejectsZeroInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.47")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/_ping") {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("OK"))
			return
		}
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("RELAYD_PROVIDER_LOCAL_TYPE", "pihole")
	t.Setenv("RELAYD_PROVIDER_LOCAL_ZONES", "home.lan")
	t.Setenv("RELAYD_PROVIDER_LOCAL_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Run(ctx, Options{Instance: "test", Interval: 0, IPFamily: "ipv4"}); err == nil {
		t.Fatal("Run with a zero interval returned nil, want an error")
	}
}
