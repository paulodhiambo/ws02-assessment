// Bonus B: RabbitMQ consumer for LoanApplicationSubmitted events.
package loanevents

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"jamiisavings/mi/tests/support"
)

func params(t *testing.T) map[string]string {
	inbound := support.Load(t, "loan-events", "inbound-endpoints", "LoanApplicationInbound")
	out := map[string]string{}
	for _, p := range inbound.All("parameter") {
		out[p.Attr("name")] = strings.TrimSpace(p.Text)
	}
	if inbound.Attr("sequence") != "loan-event-process" || inbound.Attr("onError") != "loan-event-error" {
		t.Errorf("inbound wiring: sequence=%s onError=%s", inbound.Attr("sequence"), inbound.Attr("onError"))
	}
	return out
}

// Manual ack is what makes retries and dead-lettering possible at all.
func TestManualAckAndOrderedConsumption(t *testing.T) {
	p := params(t)
	for k, want := range map[string]string{
		"rabbitmq.queue.auto.ack":                  "false",
		"rabbitmq.channel.consumer.qos":            "1",
		"rabbitmq.queue.autodeclare":               "false",
		"rabbitmq.exchange.autodeclare":            "false",
		"rabbitmq.message.content.type":            "text/plain",
		"rabbitmq.queue.name":                      "loan.applications",
		"rabbitmq.message.error.queue.routing.key": "loan.applications.dlq",
	} {
		if p[k] != want {
			t.Errorf("%s = %q, want %q", k, p[k], want)
		}
	}
}

func TestRetriesAreBounded(t *testing.T) {
	max, err := strconv.Atoi(params(t)["rabbitmq.message.max.dead.lettered.count"])
	if err != nil || max < 1 || max > 5 {
		t.Errorf("max dead-lettered count must be a small bound, got %v", params(t)["rabbitmq.message.max.dead.lettered.count"])
	}
}

func TestCredentialsAreInjected(t *testing.T) {
	p := params(t)
	for _, k := range []string{"rabbitmq.server.host.name", "rabbitmq.server.user.name", "rabbitmq.server.password"} {
		if !strings.HasPrefix(p[k], "$SYSTEM:") {
			t.Errorf("%s must come from the environment, got %q", k, p[k])
		}
	}
}

func TestClassification(t *testing.T) {
	process := support.Load(t, "loan-events", "sequences", "loan-event-process")
	// The blocking call must not turn API error statuses into faults.
	if p := process.First("property", "name", "non.error.http.status.codes"); p == nil || !strings.Contains(p.Attr("value"), "422") {
		t.Error("API error statuses must reach the switch (non.error.http.status.codes)")
	}
	if c := process.First("call"); c == nil || c.Attr("blocking") != "true" {
		t.Error("the API call must be blocking so the ack waits for the outcome")
	}
	if c := process.First("case", "regex", "4[0-9][0-9]"); c == nil || !support.Contains(c.SequenceKeys(), "loan-event-dead-letter") {
		t.Error("4xx (permanent) must be dead-lettered")
	}
	sw := process.First("switch")
	if d := sw.Child("default"); d == nil || !support.Contains(d.SequenceKeys(), "loan-event-retry") {
		t.Error("other statuses (transient) must be retried")
	}
	if onFail := process.First("on-fail"); onFail == nil || !support.Contains(onFail.SequenceKeys(), "loan-event-dead-letter") {
		t.Error("schema failures must be dead-lettered")
	}
	errSeq := support.Load(t, "loan-events", "sequences", "loan-event-error")
	if c := errSeq.First("case", "regex", "VALIDATION"); c == nil || !support.Contains(c.SequenceKeys(), "loan-event-dead-letter") {
		t.Error("unparseable bodies must be dead-lettered")
	}
}

func TestRetryRejectsAndDeadLetterKeepsTheOriginal(t *testing.T) {
	retry := support.Load(t, "loan-events", "sequences", "loan-event-retry")
	if !support.Contains(retry.PropertiesSet("SET_ROLLBACK_ONLY"), "true") {
		t.Error("retry must reject the message (SET_ROLLBACK_ONLY)")
	}
	dl := support.Load(t, "loan-events", "sequences", "loan-event-dead-letter")
	formats := dl.All("format")
	if len(formats) != 1 || !strings.Contains(dl.First("arg").Attr("expression"), "ORIGINAL_EVENT") {
		t.Error("the DLQ message must be the original event body")
	}
	headers := map[string]bool{}
	for _, h := range dl.All("header") {
		headers[h.Attr("name")] = true
	}
	for _, h := range []string{"x-error-code", "x-error-reason", "x-correlation-id"} {
		if !headers[h] {
			t.Errorf("DLQ message missing %s header", h)
		}
	}
}

func TestEventSchema(t *testing.T) {
	var schema map[string]any
	entry := support.Load(t, "loan-events", "local-entries", "LoanApplicationEventSchema")
	if err := json.Unmarshal([]byte(entry.Text), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	req := schema["required"].([]any)
	if len(req) != 4 {
		t.Errorf("required = %v", req)
	}
}
