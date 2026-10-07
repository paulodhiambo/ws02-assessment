// Cross-cutting rules every integration must follow.
package tests

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"jamiisavings/mi/tests/support"
)

// Payloads carry PII; only custom, explicitly chosen fields may be logged.
func TestNoFullPayloadLogging(t *testing.T) {
	for _, a := range support.AllArtifacts(t) {
		for _, log := range a.Root.All("log") {
			if level := log.Attr("level"); level != "custom" {
				t.Errorf("%s: log level must be 'custom', got %q", a.File, level)
			}
		}
	}
}

func TestEveryAPIResourceHasAFaultSequence(t *testing.T) {
	for _, a := range support.AllArtifacts(t) {
		for _, r := range a.Root.All("resource") {
			if r.Attr("faultSequence") == "" {
				t.Errorf("%s: resource %s without faultSequence", a.File, r.Attr("uri-template"))
			}
		}
	}
}

func TestEveryAPIUsesCorrelationAndLogging(t *testing.T) {
	for _, a := range support.AllArtifacts(t) {
		if a.Root.Name() != "api" {
			continue
		}
		keys := a.Root.SequenceKeys()
		for _, required := range []string{"correlation", "request-logging", "response-logging"} {
			if !support.Contains(keys, required) {
				t.Errorf("%s: does not call %q", a.File, required)
			}
		}
	}
}

func TestEveryEndpointHasATimeout(t *testing.T) {
	for _, a := range support.AllArtifacts(t) {
		if a.Root.Name() != "endpoint" {
			continue
		}
		timeout := a.Root.First("timeout")
		if timeout == nil || timeout.Child("duration") == nil {
			t.Errorf("%s: endpoint without timeout", a.File)
			continue
		}
		ms, err := strconv.Atoi(strings.TrimSpace(timeout.Child("duration").Text))
		if err != nil || ms > 10000 {
			t.Errorf("%s: timeout must be a number of ms <= 10000, got %q", a.File, timeout.Child("duration").Text)
		}
	}
}

// Fault sequences must delegate to common-error so every API shares one error shape.
func TestErrorResponsesOnlyComeFromCommonError(t *testing.T) {
	for _, a := range support.AllArtifacts(t) {
		if a.Root.Name() != "sequence" || !strings.HasSuffix(a.Root.Attr("name"), "-fault") {
			continue
		}
		if !support.Contains(a.Root.SequenceKeys(), "common-error") {
			t.Errorf("%s: fault sequence must call common-error", a.File)
		}
		if f := a.Root.PayloadFormats(); len(f) != 0 {
			t.Errorf("%s: fault sequence builds its own payload", a.File)
		}
	}
}

func TestCommonErrorEnvelopeShape(t *testing.T) {
	root := support.Load(t, "common", "sequences", "common-error")
	formats := root.PayloadFormats()
	if len(formats) != 1 {
		t.Fatalf("want exactly one payload format, got %d", len(formats))
	}
	body := support.JSONFormat(t, formats[0])
	if got, want := support.Keys(body), []string{"correlationId", "error", "requestId", "timestamp"}; !slices.Equal(got, want) {
		t.Errorf("envelope keys = %v, want %v", got, want)
	}
	if got, want := support.Keys(body["error"]), []string{"code", "message", "status"}; !slices.Equal(got, want) {
		t.Errorf("error keys = %v, want %v", got, want)
	}
	for _, key := range support.ErrorEnvelopeKeys {
		if !strings.Contains(formats[0], `"`+key+`"`) {
			t.Errorf("format does not contain %q", key)
		}
	}
}

func TestCommonErrorDropsBackendHeaders(t *testing.T) {
	root := support.Load(t, "common", "sequences", "common-error")
	if root.First("property", "name", "TRANSPORT_HEADERS", "action", "remove") == nil {
		t.Error("common-error must remove TRANSPORT_HEADERS")
	}
}

// mask re-implements the mask-sensitive-data XPath to pin its behaviour.
func mask(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
}

func TestMaskingKeepsLastFour(t *testing.T) {
	root := support.Load(t, "common", "sequences", "mask-sensitive-data")
	expr := root.First("then").Child("property").Attr("expression")
	if !strings.Contains(expr, "string-length(get-property('MASK_INPUT')) - 3") {
		t.Errorf("unexpected masking expression: %s", expr)
	}
	if got := mask("0100000001"); got != "******0001" {
		t.Errorf("mask(0100000001) = %s", got)
	}
	if got := mask("42"); got != "****" {
		t.Errorf("mask(42) = %s", got)
	}
}
