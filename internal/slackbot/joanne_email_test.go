package slackbot

import "testing"

func TestNormalizeEmailAddress(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"grant@bimross.com", "grant@bimross.com"},
		{"<mailto:grant@bimross.com|grant@bimross.com>", "grant@bimross.com"},
		{"to: grant@bimross.com;", "grant@bimross.com"},
		{" grant@bimross.com, ", "grant@bimross.com"},
	}
	for _, tc := range tests {
		if got := normalizeEmailAddress(tc.in); got != tc.want {
			t.Fatalf("normalizeEmailAddress(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
