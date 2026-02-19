package slug

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Morning Intel Brief", "morning-intel-brief"},
		{"sam-gov — search_opportunities", "sam-gov-search-opportunities"},
		{"simple", "simple"},
		{"UPPER-case", "upper-case"},
		{"lots   of   spaces", "lots-of-spaces"},
		{"trailing-dash-", "trailing-dash"},
		{"-leading-dash", "leading-dash"},
		{"special!@#chars", "special-chars"},
		{"under_score", "under-score"},
		{"", "unknown"},
		{"!@#$%", "unknown"},
		{"---", "unknown"},
	}
	for _, tc := range tests {
		got := Sanitize(tc.in)
		if got != tc.want {
			t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
