package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Special ids that simulate backend failure modes, so the gateway's
// timeout and fault handling can be exercised deterministically.
const (
	slowID  = "999" // responds after slowDelay (beyond MI's 5s endpoint timeout)
	errorID = "500" // responds 500
)

var userPath = regexp.MustCompile(`^/users/([^/]+)$`)

func newHandler(slowDelay time.Duration, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" { // not logged: probed every 10s by the healthcheck
			send(w, http.StatusOK, map[string]string{"status": "UP"})
			return
		}
		logRequest(logger, r)

		m := userPath.FindStringSubmatch(r.URL.Path)
		if r.Method != http.MethodGet || m == nil {
			send(w, http.StatusNotFound, struct{}{})
			return
		}

		switch id := m[1]; id {
		case errorID:
			send(w, http.StatusInternalServerError, map[string]string{"message": "Internal Server Error"})
		case slowID:
			select {
			case <-time.After(slowDelay):
				send(w, http.StatusOK, users[0])
			case <-r.Context().Done(): // caller (MI) gave up
			}
		default:
			for _, u := range users {
				if strconv.Itoa(u.ID) == id {
					send(w, http.StatusOK, u)
					return
				}
			}
			// JSONPlaceholder returns 404 with an empty object for unknown ids.
			send(w, http.StatusNotFound, struct{}{})
		}
	})
}

func send(w http.ResponseWriter, status int, body any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	// Internal headers a real backend might leak; the gateway must strip them.
	h.Set("X-Powered-By", "Express")
	h.Set("X-Backend-Node", "customer-svc-02")
	h.Set("Server", "mock-customer-service/1.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// logRequest logs the headers actually received, so a demo can show what the
// gateway injected (correlation id) and stripped (internal headers).
func logRequest(logger *log.Logger, r *http.Request) {
	headers := map[string]string{"host": r.Host}
	for name, values := range r.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	line, _ := json.Marshal(map[string]any{"method": r.Method, "path": r.URL.Path, "headers": headers})
	logger.Println(string(line))
}
