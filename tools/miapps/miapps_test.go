package miapps

import (
	"strings"
	"testing"
)

const resp = `{"activeCount":2,"faultyCount":1,
  "activeList":[{"name":"JamiiCommon","version":"1.0.1"},{"name":"JamiiAccountBalance","version":"1.0.0"}],
  "faultyList":["JamiiLoanEligibility_1.0.1.car"]}`

func TestState(t *testing.T) {
	cases := []struct {
		expected []string
		want     string
	}{
		{[]string{"JamiiCommon:1.0.1"}, "ok"},
		{[]string{"JamiiCommon:1.0.1", "JamiiAccountBalance:1.0.1"}, "pending:JamiiAccountBalance:1.0.1"},
		{[]string{"JamiiCommon:1.0.1", "JamiiLoanEligibility:1.0.1"}, "faulty:JamiiLoanEligibility"},
	}
	for _, c := range cases {
		got, err := State(strings.NewReader(resp), c.expected)
		if err != nil || got != c.want {
			t.Errorf("State(%v) = %q, %v; want %q", c.expected, got, err, c.want)
		}
	}
}

func TestUndeployedIgnoresVersion(t *testing.T) {
	if done, _ := Undeployed(strings.NewReader(resp), []string{"JamiiCommon:9.9.9"}); done {
		t.Error("JamiiCommon is still active (another version), so not undeployed")
	}
	if done, _ := Undeployed(strings.NewReader(resp), []string{"JamiiCustomerProxy:1.0.0"}); !done {
		t.Error("JamiiCustomerProxy is not active, so it is undeployed")
	}
}

func TestActive(t *testing.T) {
	if got, _ := Active(strings.NewReader(resp)); got != "JamiiCommon:1.0.1, JamiiAccountBalance:1.0.0" {
		t.Errorf("Active = %q", got)
	}
}
