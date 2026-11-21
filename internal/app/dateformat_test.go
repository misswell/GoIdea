package app

import (
	"testing"
	"time"
)

func TestDateFormatPatternUsesOpenIDEAPlaceholders(t *testing.T) {
	when := time.Date(2026, time.November, 9, 15, 7, 0, 0, time.Local)
	tests := map[string]string{
		"yyyy/MM/dd":          "2026/11/09",
		"yy-M-d":              "26-11-9",
		"dd MMM yyyy":         "09 Nov 2026",
		"MMMM d, yy":          "November 9, 26",
		"literal-yyyy-MM-dd!": "literal-2026-11-09!",
	}
	for pattern, want := range tests {
		if got := formatDatePattern(when, pattern); got != want {
			t.Fatalf("pattern %q = %q, want %q", pattern, got, want)
		}
	}
}

func TestDateFormatPrefsDefaultsAndStoredOverride(t *testing.T) {
	if got := (Preferences{}).dateFormatPrefs(); got != defaultDateFormatPrefs() {
		t.Fatalf("default date format = %+v", got)
	}
	stored := DateFormatPrefs{
		OverrideSystem: true, Pattern: "  yyyy.MM.dd  ", Use24Hour: false, Pretty: false,
	}
	got := (Preferences{DateFormat: &stored}).dateFormatPrefs()
	if got.Pattern != "yyyy.MM.dd" || !got.OverrideSystem || got.Use24Hour || got.Pretty {
		t.Fatalf("stored date format = %+v", got)
	}
	if ptr := dateFormatPrefsPointer(defaultDateFormatPrefs()); ptr != nil {
		t.Fatalf("defaults were persisted as %+v", ptr)
	}
}

func TestFormatAppTimestampPrettyAndClock(t *testing.T) {
	now := time.Date(2026, time.November, 21, 15, 30, 0, 0, time.Local)
	prefs := DateFormatPrefs{OverrideSystem: true, Pattern: "yyyy-MM-dd", Use24Hour: true, Pretty: true}
	tests := []struct {
		when time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "Moments ago"},
		{now.Add(-5 * time.Minute), "5 min ago"},
		{now.Add(-2 * time.Hour), "Today 13:30"},
		{now.Add(-24 * time.Hour), "Yesterday 15:30"},
		{now.AddDate(0, 0, -5), "2026-11-16"},
	}
	for _, test := range tests {
		if got := formatAppTimestamp(prefs, dateRegion{}, float64(test.when.UnixMilli()), now); got != test.want {
			t.Fatalf("timestamp %s = %q, want %q", test.when, got, test.want)
		}
	}

	prefs.Pretty = false
	prefs.Use24Hour = false
	if got := formatAppTimestamp(prefs, dateRegion{}, float64(now.UnixMilli()), now); got != "2026-11-21 03:30 PM" {
		t.Fatalf("plain 12-hour timestamp = %q", got)
	}
}

func TestSystemDateFormatMatchesIntlRegionalPatterns(t *testing.T) {
	when := time.Date(2026, time.November, 9, 15, 7, 0, 0, time.Local)
	tests := []struct {
		locale    string
		hourCycle int
		date      string
		short     string
		clock     string
	}{
		{"en-US", 12, "Nov 9, 2026", "11/09/26", "03:07 PM"},
		{"en-GB", 24, "9 Nov 2026", "09/11/26", "15:07"},
		{"de-DE", 24, "9. Nov. 2026", "09.11.26", "15:07"},
		{"zh-CN", 24, "2026年11月9日", "26/11/09", "15:07"},
	}
	for _, test := range tests {
		if got := formatSystemDate(test.locale, test.hourCycle, when, false); got != test.date {
			t.Errorf("%s date = %q, want %q", test.locale, got, test.date)
		}
		if got := formatSystemDate(test.locale, test.hourCycle, when, true); got != test.short {
			t.Errorf("%s short date = %q, want %q", test.locale, got, test.short)
		}
		if got := formatSystemClock(test.locale, test.hourCycle, when); got != test.clock {
			t.Errorf("%s clock = %q, want %q", test.locale, got, test.clock)
		}
	}
}
