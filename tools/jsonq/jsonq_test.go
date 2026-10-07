package jsonq

import (
	"slices"
	"strings"
	"testing"
)

const doc = `{
  "AccessToken": "abc",
  "partial": true,
  "error": {"code": "ACCOUNT_NOT_FOUND", "status": 404},
  "errors": [{"section": "accounts", "code": "ACCOUNT_DB_UNAVAILABLE"}],
  "values": [{"key": "accessToken", "value": "t1"}, {"key": "apiKey", "value": "k1"}],
  "list": [{"name": "A", "url": "http://a"}, {"name": "B", "url": "http://b"}],
  "amount": 152340.75
}`

func TestQuery(t *testing.T) {
	cases := map[string][]string{
		"AccessToken":              {"abc"},
		"partial":                  {"true"},
		"error.code":               {"ACCOUNT_NOT_FOUND"},
		"error.status":             {"404"},
		"errors[0].code":           {"ACCOUNT_DB_UNAVAILABLE"},
		"values[key=apiKey].value": {"k1"},
		"list[*].url":              {"http://a", "http://b"},
		"amount":                   {"152340.75"},
		"error":                    {`{"code":"ACCOUNT_NOT_FOUND","status":404}`},
	}
	for path, want := range cases {
		got, err := Query(strings.NewReader(doc), path)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: got %v (%v), want %v", path, got, err, want)
		}
	}
}

func TestMissingPathAndBadJSON(t *testing.T) {
	if _, err := Query(strings.NewReader(doc), "error.nope"); err == nil {
		t.Error("missing path should fail")
	}
	if _, err := Query(strings.NewReader("<html>"), "a"); err == nil {
		t.Error("non-JSON input should fail")
	}
}
