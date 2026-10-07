package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(newHandler(50*time.Millisecond, log.New(io.Discard, "", 0)))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func TestKnownUser(t *testing.T) {
	resp, body := get(t, newTestServer(t).URL+"/users/1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var u user
	if err := json.Unmarshal(body, &u); err != nil {
		t.Fatal(err)
	}
	if u.ID != 1 || u.Email == "" {
		t.Fatalf("unexpected user %+v", u)
	}
	if got := resp.Header.Get("X-Powered-By"); got == "" {
		t.Fatal("expected leaky backend headers for the gateway to strip")
	}
}

func TestUnknownUserIs404WithEmptyObject(t *testing.T) {
	resp, body := get(t, newTestServer(t).URL+"/users/42")
	if resp.StatusCode != http.StatusNotFound || string(body) != "{}\n" {
		t.Fatalf("got %d %q", resp.StatusCode, body)
	}
}

func TestSimulatedBackendError(t *testing.T) {
	resp, _ := get(t, newTestServer(t).URL+"/users/500")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestSlowUserRespondsAfterDelay(t *testing.T) {
	start := time.Now()
	resp, _ := get(t, newTestServer(t).URL+"/users/999")
	if resp.StatusCode != http.StatusOK || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("status %d after %v", resp.StatusCode, time.Since(start))
	}
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	if code := probe(srv.URL + "/health"); code != 0 {
		t.Fatalf("probe returned %d", code)
	}
}
