package loaneligibility

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"jamiisavings/mi/tests/support"
)

// decide is the reference implementation of the mapping documented in
// docs/api-design.md.
func decide(monthlyIncome, existingDebt, requestedAmount float64, tenureMonths int) (installment, maxAffordable float64, eligible bool) {
	installment = math.RoundToEven(math.Round(requestedAmount) / float64(tenureMonths)) // the SOAP service rounds half-to-even
	maxAffordable = math.RoundToEven((monthlyIncome*0.4-existingDebt)*100) / 100
	return installment, maxAffordable, installment <= maxAffordable
}

type fixture struct {
	api, fault *support.Node
	schema     map[string]any
}

func load(t *testing.T) fixture {
	f := fixture{
		api:   support.Load(t, "loan-eligibility", "api", "LoanEligibilityAPI"),
		fault: support.Load(t, "loan-eligibility", "sequences", "loan-fault"),
	}
	entry := support.Load(t, "loan-eligibility", "local-entries", "LoanEligibilityRequestSchema")
	if err := json.Unmarshal([]byte(entry.Text), &f.schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return f
}

func TestSchemaRequiresCoreFields(t *testing.T) {
	s := load(t).schema
	var required []string
	for _, r := range s["required"].([]any) {
		required = append(required, r.(string))
	}
	slices.Sort(required)
	if want := []string{"customerId", "monthlyIncome", "requestedAmount", "tenureMonths"}; !slices.Equal(required, want) {
		t.Errorf("required = %v, want %v", required, want)
	}
	if s["additionalProperties"] != false {
		t.Error("additionalProperties must be false")
	}
	tenure := s["properties"].(map[string]any)["tenureMonths"].(map[string]any)
	if tenure["type"] != "integer" {
		t.Error("tenureMonths must be an integer")
	}
}

func TestValidateMediatorUsesSchemaAndReturns400(t *testing.T) {
	validate := load(t).api.First("validate")
	if validate.Child("schema").Attr("key") != "LoanEligibilityRequestSchema" {
		t.Error("validate must use LoanEligibilityRequestSchema")
	}
	if !support.Contains(validate.Child("on-fail").PropertiesSet("ERROR_STATUS"), "400") {
		t.Error("schema failures must be 400")
	}
}

func TestSOAPFaultsAreMappedNotLeaked(t *testing.T) {
	filter := load(t).api.First("filter", "xpath", "boolean($body/soap:Fault)")
	if filter == nil {
		t.Fatal("no SOAP fault branch")
	}
	branch := filter.Child("then")
	if got := support.Set(branch.PropertiesSet("ERROR_STATUS")...); !slices.Equal(got, []string{"422", "502"}) {
		t.Errorf("SOAP fault statuses = %v, want [422 502]", got)
	}
	if len(branch.PayloadFormats()) != 0 {
		t.Error("fault branch must not build a payload from the SOAP fault")
	}
	if !support.Contains(branch.SequenceKeys(), "common-error") {
		t.Error("fault branch must use common-error")
	}
}

func TestSuccessResponseShape(t *testing.T) {
	formats := load(t).api.PayloadFormats()
	if len(formats) != 1 {
		t.Fatalf("want one payload format, got %d", len(formats))
	}
	body := support.JSONFormat(t, formats[0])
	if got, want := support.Keys(body), []string{"customerId", "decision", "eligible", "loan", "reason", "requestId", "timestamp"}; !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if got, want := support.Keys(body["loan"]), []string{"currency", "maxAffordableInstallment", "monthlyInstallment", "requestedAmount", "tenureMonths"}; !slices.Equal(got, want) {
		t.Errorf("loan keys = %v, want %v", got, want)
	}
}

func TestTransportFaultMapping(t *testing.T) {
	statuses := load(t).fault.PropertiesSet("ERROR_STATUS")
	for _, s := range []string{"400", "500", "502", "503", "504"} {
		if !support.Contains(statuses, s) {
			t.Errorf("loan-fault does not map to %s", s)
		}
	}
}

// Values verified against the live service and MI (see tests/integration/loan-eligibility.http).
func TestDecisionRuleExamples(t *testing.T) {
	cases := []struct {
		income, debt, amount float64
		tenure               int
		installment, max     float64
		eligible             bool
	}{
		{120000, 15000, 500000, 12, 41667, 33000, false},
		{120000, 0, 300000, 12, 25000, 48000, true},
		{2, 0, 5, 2, 2, 0.8, false},
	}
	for _, c := range cases {
		i, m, e := decide(c.income, c.debt, c.amount, c.tenure)
		if i != c.installment || m != c.max || e != c.eligible {
			t.Errorf("decide(%v) = (%v, %v, %v), want (%v, %v, %v)", c, i, m, e, c.installment, c.max, c.eligible)
		}
	}
}
