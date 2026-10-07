package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func call(t *testing.T, op string, a, b any) (int, string) {
	t.Helper()
	srv := httptest.NewServer(newHandler(log.New(io.Discard, "", 0)))
	defer srv.Close()
	body := fmt.Sprintf(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
		`<%s xmlns="http://tempuri.org/"><intA>%v</intA><intB>%v</intB></%s></soap:Body></soap:Envelope>`, op, a, b, op)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/calculator.asmx", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"http://tempuri.org/`+op+`"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func TestDivideRoundsHalfToEvenLikeDotNet(t *testing.T) {
	cases := []struct{ a, b, want int }{{500000, 12, 41667}, {5, 2, 2}, {7, 2, 4}}
	for _, c := range cases {
		status, body := call(t, "Divide", c.a, c.b)
		want := fmt.Sprintf("<DivideResult>%d</DivideResult>", c.want)
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("Divide(%d,%d): got %d %s, want %s", c.a, c.b, status, body, want)
		}
	}
}

func TestMultiply(t *testing.T) {
	if status, body := call(t, "Multiply", 6, 7); status != http.StatusOK || !strings.Contains(body, "<MultiplyResult>42</MultiplyResult>") {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestInt32OverflowIsClientFault(t *testing.T) {
	status, body := call(t, "Divide", int64(3000000000), 12)
	if status != http.StatusInternalServerError || !strings.Contains(body, "<faultcode>soap:Client</faultcode>") {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestDivideByZeroIsServerFault(t *testing.T) {
	status, body := call(t, "Divide", 10, 0)
	if status != http.StatusInternalServerError || !strings.Contains(body, "<faultcode>soap:Server</faultcode>") {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestNonNumericInputIsClientFault(t *testing.T) {
	status, body := call(t, "Divide", "abc", 1)
	if status != http.StatusInternalServerError || !strings.Contains(body, "Input string was not in a correct format") {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestWSDLIsServed(t *testing.T) {
	srv := httptest.NewServer(newHandler(log.New(io.Discard, "", 0)))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/calculator.asmx?WSDL")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `soapAction="http://tempuri.org/Divide"`) {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if probe(srv.URL+"/health") != 0 {
		t.Fatal("health probe failed")
	}
}
