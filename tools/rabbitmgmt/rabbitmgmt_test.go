package rabbitmgmt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublishGetPurge(t *testing.T) {
	var published map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, _ := r.BasicAuth(); u != "jamii" || p != "secret" {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.EscapedPath() == "/api/exchanges/%2F/loan.events/publish":
			_ = json.NewDecoder(r.Body).Decode(&published)
			_, _ = w.Write([]byte(`{"routed":true}`))
		case r.Method == "POST" && r.URL.EscapedPath() == "/api/queues/%2F/loan.decisions/get":
			_, _ = w.Write([]byte(`[{"payload":"{\"a\":1}","redelivered":false,"properties":{"headers":{"x-correlation-id":"c1"}}}]`))
		case r.Method == "DELETE" && r.URL.EscapedPath() == "/api/queues/%2F/loan.decisions/contents":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, r.Method+" "+r.URL.EscapedPath(), http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "jamii", "secret")

	if err := c.Publish("loan.events", "loan.applications", `{"x":1}`, "", map[string]string{"x-correlation-id": "c1"}); err != nil {
		t.Fatal(err)
	}
	if published["routing_key"] != "loan.applications" || published["payload"] != `{"x":1}` {
		t.Errorf("unexpected publish body %v", published)
	}
	if _, set := published["properties"].(map[string]any)["content_type"]; set {
		t.Error("content_type must be omitted when empty")
	}
	msgs, err := c.Get("loan.decisions", 5)
	if err != nil || len(msgs) != 1 || msgs[0].Headers["x-correlation-id"] != "c1" {
		t.Fatalf("got %+v, %v", msgs, err)
	}
	if err := c.Purge("loan.decisions"); err != nil {
		t.Fatal(err)
	}
}

func TestUnroutedPublishIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"routed":false}`))
	}))
	defer srv.Close()
	err := New(srv.URL, "u", "p").Publish("loan.events", "nowhere", "x", "", nil)
	if err == nil || !strings.Contains(err.Error(), "not routed") {
		t.Fatalf("want not-routed error, got %v", err)
	}
}
