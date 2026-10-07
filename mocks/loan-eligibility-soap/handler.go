package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

//go:embed wsdl/loan-eligibility.wsdl
var wsdl []byte

const envelopeOpen = `<?xml version="1.0" encoding="utf-8"?>` +
	`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" ` +
	`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">` +
	`<soap:Body>`

const envelopeClose = `</soap:Body></soap:Envelope>`

// Operation and field patterns, compiled once. Matching ignores namespace
// prefixes; good enough for a mock.
var (
	opPatterns    = map[string]*regexp.Regexp{}
	fieldPatterns = map[string]*regexp.Regexp{}
	opNames       []string
)

func init() {
	for name := range operations {
		opNames = append(opNames, name)
		opPatterns[name] = regexp.MustCompile(`<(?:\w+:)?` + name + `[\s>]`)
	}
	sort.Strings(opNames)
	for _, f := range []string{"intA", "intB"} {
		fieldPatterns[f] = regexp.MustCompile(`<(?:\w+:)?` + f + `>([^<]*)</(?:\w+:)?` + f + `>`)
	}
}

type soapRequest struct {
	Op         string
	IntA, IntB *string
}

func parseRequest(xml string) soapRequest {
	var req soapRequest
	for _, name := range opNames {
		if opPatterns[name].MatchString(xml) {
			req.Op = name
			break
		}
	}
	field := func(name string) *string {
		if m := fieldPatterns[name].FindStringSubmatch(xml); m != nil {
			return &m[1]
		}
		return nil
	}
	req.IntA, req.IntB = field("intA"), field("intB")
	return req
}

func newHandler(logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"UP"}`)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/calculator.asmx" && strings.EqualFold(r.URL.RawQuery, "wsdl"):
			w.Header().Set("Content-Type", "text/xml; charset=utf-8")
			_, _ = w.Write(wsdl)
			return
		case r.Method != http.MethodPost || r.URL.Path != "/calculator.asmx":
			w.WriteHeader(http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeFault(w, clientFault("Server was unable to read request."))
			return
		}
		req := parseRequest(string(body))
		logRequest(logger, r.Header.Get("SOAPAction"), req)

		result, err := invoke(req)
		if err != nil {
			var fault *soapFault
			if !errors.As(err, &fault) {
				fault = serverFault(err.Error())
			}
			writeFault(w, fault)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprintf(w, `%s<%sResponse xmlns="http://tempuri.org/"><%sResult>%d</%sResult></%sResponse>%s`,
			envelopeOpen, req.Op, req.Op, result, req.Op, req.Op, envelopeClose)
	})
}

func invoke(req soapRequest) (int64, error) {
	op, ok := operations[req.Op]
	if !ok {
		return 0, clientFault("Server did not recognize the value of HTTP Header SOAPAction.")
	}
	a, err := parseInt32(req.IntA, "intA")
	if err != nil {
		return 0, err
	}
	b, err := parseInt32(req.IntB, "intB")
	if err != nil {
		return 0, err
	}
	return op(a, b)
}

// Like the real .NET service, the fault string includes a (fake) stack trace:
// the gateway must never pass this through to API consumers.
func writeFault(w http.ResponseWriter, f *soapFault) {
	faultString := "System.Web.Services.Protocols.SoapException: " + f.Message +
		"\n   at Calculator.Invoke() in C:\\inetpub\\calculator\\Calculator.asmx.cs:line 42"
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `%s<soap:Fault><faultcode>%s</faultcode><faultstring>%s</faultstring><detail /></soap:Fault>%s`,
		envelopeOpen, f.Code, html.EscapeString(faultString), envelopeClose)
}

func logRequest(logger *log.Logger, soapAction string, req soapRequest) {
	entry := map[string]any{"soapAction": soapAction, "op": req.Op, "intA": req.IntA, "intB": req.IntB}
	line, _ := json.Marshal(entry)
	logger.Println(string(line))
}
