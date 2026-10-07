// Offline, contract-compatible stand-in for the DNE Online SOAP calculator.
//
//	POST /calculator.asmx        Multiply, Divide (SOAP 1.1)
//	GET  /calculator.asmx?WSDL   the contract (embedded in the binary)
//	PORT                         listen port (default 8088)
//
// Run with -healthcheck to probe a running instance (used by the Docker
// HEALTHCHECK, since the scratch image has no curl/wget).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /health on the local instance and exit 0 if it is up")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}
	if *healthcheck {
		os.Exit(probe("http://127.0.0.1:" + port + "/health"))
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(log.New(os.Stdout, "", 0)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("loan-eligibility SOAP mock listening on :%s (POST /calculator.asmx, GET /calculator.asmx?WSDL)", port)
	log.Fatal(srv.ListenAndServe())
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
