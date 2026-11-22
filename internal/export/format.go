package export

import (
	"strconv"
	"strings"
	"time"
)

// formatNumber renders a float without trailing zeros or exponent noise.
func formatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	// Guard against "-0".
	if s == "-0" {
		return "0"
	}
	return s
}

// formatDay renders a timestamp as a calendar date in UTC. Environmental dates
// are stored as UTC midnights, so formatting must not apply a local offset.
func formatDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// formatDayCompact renders YYYCMMDD in UTC (AQS transaction dates).
func formatDayCompact(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("20060102")
}

// formatDate renders YYYY-MM-DD.
func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// formatDateTime renders ISO-8601 with timezone.
func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// xmlEscape escapes text content for XML output.
func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeXML strips characters that are illegal in XML 1.0.
func sanitizeXML(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0x9 || r == 0xA || r == 0xD:
			b.WriteRune(r)
		case r >= 0x20 && r <= 0xD7FF:
			b.WriteRune(r)
		case r >= 0xE000 && r <= 0xFFFD:
			b.WriteRune(r)
		case r >= 0x10000 && r <= 0x10FFFF:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// xmlText escapes and sanitizes.
func xmlText(s string) string { return xmlEscape(sanitizeXML(s)) }

// uniqueStrings preserves first-seen order while de-duplicating.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// detectFlagCode maps Samplier's Y/N flag to a regulatory code.
func detectFlagCode(row Row) string {
	if row.IsNonDetect() {
		return "N"
	}
	if row.ResultNumeric != nil || row.ResultText != "" {
		return "Y"
	}
	return ""
}
