// Offline stand-in for JSONPlaceholder /users/{id}, with failure modes for testing.
//
//	PORT           listen port (default 3000)
//	SLOW_DELAY_MS  delay for /users/999 (default 15000)
//
// Run with -healthcheck to probe a running instance (used by the Docker
// HEALTHCHECK, since the scratch image has no curl/wget).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /health on the local instance and exit 0 if it is up")
	flag.Parse()

	port := envOr("PORT", "3000")
	if *healthcheck {
		os.Exit(probe("http://127.0.0.1:" + port + "/health"))
	}

	slowMs, err := strconv.Atoi(envOr("SLOW_DELAY_MS", "15000"))
	if err != nil {
		log.Fatalf("invalid SLOW_DELAY_MS: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(time.Duration(slowMs)*time.Millisecond, log.New(os.Stdout, "", 0)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("customer-service mock listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func probe(url string) int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
