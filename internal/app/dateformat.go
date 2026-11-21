package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
)

const defaultDatePattern = "dd.MM.yyyy"

// dateRegion is the regional formatting snapshot captured from the UI context
// on the main thread. Formatting helpers are also used by tests and background
// callbacks, so they must never call mygo.App.Region themselves.
type dateRegion = i18n.Region

// DateFormatPrefs is Settings › System Settings › Date Formats. The pointer in
// Preferences is nil while every choice is OpenIDEA's default, so older files
// keep working without a migration.
type DateFormatPrefs struct {
	OverrideSystem bool   `json:"overrideSystem,omitempty"`
	Pattern        string `json:"pattern,omitempty"`
	Use24Hour      bool   `json:"use24Hour"`
	Pretty         bool   `json:"pretty"`
}

func defaultDateFormatPrefs() DateFormatPrefs {
	return DateFormatPrefs{
		Pattern:   defaultDatePattern,
		Use24Hour: true,
		Pretty:    true,
	}
}

// dateFormatPrefs returns the effective choices. A missing stored value and an
// empty pattern both mean OpenIDEA's defaults.
func (p Preferences) dateFormatPrefs() DateFormatPrefs {
	out := defaultDateFormatPrefs()
	if p.DateFormat != nil {
		out = *p.DateFormat
	}
	out.Pattern = strings.TrimSpace(out.Pattern)
	if out.Pattern == "" {
		out.Pattern = defaultDatePattern
	}
	return out
}

func dateFormatPrefsPointer(p DateFormatPrefs) *DateFormatPrefs {
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Pattern == "" {
		p.Pattern = defaultDatePattern
	}
	if p == defaultDateFormatPrefs() {
		return nil
	}
	return &p
}

func sameDateFormatPrefs(a, b DateFormatPrefs) bool {
	a.Pattern = strings.TrimSpace(a.Pattern)
	if a.Pattern == "" {
		a.Pattern = defaultDatePattern
	}
	b.Pattern = strings.TrimSpace(b.Pattern)
	if b.Pattern == "" {
		b.Pattern = defaultDatePattern
	}
	return a == b
}

// formatDatePattern implements OpenIDEA's dateFormat.ts placeholders. Month
// names stay English, as upstream's formatter explicitly uses that locale.
func formatDatePattern(t time.Time, pattern string) string {
	var b strings.Builder
	for at := 0; at < len(pattern); {
		matched := false
		for _, token := range []string{"yyyy", "MMMM", "MMM", "yy", "MM", "dd", "M", "d"} {
			if !strings.HasPrefix(pattern[at:], token) {
				continue
			}
			switch token {
			case "yyyy":
				fmt.Fprintf(&b, "%04d", t.Year())
			case "yy":
				fmt.Fprintf(&b, "%02d", t.Year()%100)
			case "MMMM":
				b.WriteString(t.Format("January"))
			case "MMM":
				b.WriteString(t.Format("Jan"))
			case "MM":
				fmt.Fprintf(&b, "%02d", int(t.Month()))
			case "M":
				fmt.Fprintf(&b, "%d", int(t.Month()))
			case "dd":
				fmt.Fprintf(&b, "%02d", t.Day())
			case "d":
				fmt.Fprintf(&b, "%d", t.Day())
			}
			at += len(token)
			matched = true
			break
		}
		if matched {
			continue
		}
		b.WriteByte(pattern[at])
		at++
	}
	return b.String()
}

func formatAppDate(p DateFormatPrefs, r dateRegion, ms float64, short bool) string {
	if ms <= 0 {
		return ""
	}
	t := time.UnixMilli(int64(ms))
	if p.OverrideSystem {
		pattern := strings.TrimSpace(p.Pattern)
		if pattern == "" {
			pattern = defaultDatePattern
		}
		return formatDatePattern(t, pattern)
	}
	return formatSystemDate(r.Locale, r.HourCycle, t, short)
}

// formatSystemDate follows OpenIDEA's Intl.DateTimeFormat(undefined,
// options) calls. MyGo exposes the same system region, while i18n applies
// its CLDR patterns and names instead of hard-coding an English layout.
func formatSystemDate(locale string, hourCycle int, t time.Time, short bool) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "en"
	}
	skeleton := "yMMMd"
	if short {
		skeleton = "yyMMdd"
	}
	return i18n.FormatDate(locale, t, i18n.DateOptions{
		Skeleton:  skeleton,
		HourCycle: hourCycle,
	})
}

func formatAppClock(p DateFormatPrefs, r dateRegion, t time.Time) string {
	if !p.OverrideSystem {
		return formatSystemClock(r.Locale, r.HourCycle, t)
	}
	if p.Use24Hour {
		return t.Format("15:04")
	}
	return t.Format("03:04 PM")
}

func formatSystemClock(locale string, hourCycle int, t time.Time) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "en"
	}
	if hourCycle != 12 && hourCycle != 24 {
		hourCycle = i18n.HourCycle(locale)
	}
	skeleton := "HHmm"
	if hourCycle == 12 {
		// Intl's hour "2-digit" option pads the hour and keeps the
		// locale's day period around that padded field.
		skeleton = "hhmma"
	}
	return i18n.FormatDate(locale, t, i18n.DateOptions{
		Skeleton:  skeleton,
		HourCycle: hourCycle,
	})
}

func formatAppTimestamp(p DateFormatPrefs, r dateRegion, ms float64, now time.Time) string {
	if ms <= 0 {
		return ""
	}
	t := time.UnixMilli(int64(ms))
	if !p.Pretty {
		return formatAppDate(p, r, ms, false) + " " + formatAppClock(p, r, t)
	}
	diff := now.Sub(t)
	switch {
	case diff >= 0 && diff < time.Minute:
		return "Moments ago"
	case diff >= 0 && diff < time.Hour:
		return fmt.Sprintf("%d min ago", int(diff/time.Minute))
	}
	if sameCalendarDay(t, now) {
		return "Today " + formatAppClock(p, r, t)
	}
	if sameCalendarDay(t, now.AddDate(0, 0, -1)) {
		return "Yesterday " + formatAppClock(p, r, t)
	}
	return formatAppDate(p, r, ms, false)
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
