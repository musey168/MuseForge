package main

import (
	"reflect"
	"testing"
)

func TestParseLabels(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{" , ,", nil},
		{"gpu", []string{"gpu"}},
		{" gpu , linux,,arm64 ", []string{"gpu", "linux", "arm64"}},
	}
	for _, tc := range cases {
		if got := parseLabels(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseLabels(%q) = %#v, want %#v", tc.raw, got, tc.want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", " value "); got != "value" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", "   "); got != "" {
		t.Fatalf("firstNonEmpty = %q, want empty", got)
	}
}
