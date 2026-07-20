package models

import "testing"

func TestIsValidRuleType(t *testing.T) {
	valid := []string{"BINARY", "CERTIFICATE", "SIGNINGID", "TEAMID", "CDHASH"}
	for _, rt := range valid {
		if !IsValidRuleType(rt) {
			t.Errorf("expected %q to be valid", rt)
		}
	}

	invalid := []string{"", "binary", "TEAM", "FOO", "TEAMID "}
	for _, rt := range invalid {
		if IsValidRuleType(rt) {
			t.Errorf("expected %q to be invalid", rt)
		}
	}
}
