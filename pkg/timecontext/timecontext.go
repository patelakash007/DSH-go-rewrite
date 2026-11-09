package timecontext

import (
	"strings"
	"time"
)

func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d / time.Second)
	days := seconds / 86400
	seconds %= 86400
	hours := seconds / 3600
	seconds %= 3600
	minutes := seconds / 60
	seconds %= 60
	var parts []string
	if days > 0 {
		parts = append(parts, itoa(days)+"d")
	}
	if hours > 0 {
		parts = append(parts, itoa(hours)+"h")
	}
	if minutes > 0 {
		parts = append(parts, itoa(minutes)+"m")
	}
	parts = append(parts, itoa(seconds)+"s")
	return strings.Join(parts, " ")
}

func Render(now time.Time, loc *time.Location, previous *time.Time) string {
	if loc == nil {
		loc = time.UTC
	}
	elapsed := "unavailable"
	if previous != nil {
		elapsed = FormatElapsed(now.Sub(*previous))
	}
	var b strings.Builder
	b.WriteString("Current sampled time: ")
	b.WriteString(now.In(loc).Format("2006-01-02 15:04:05 MST"))
	b.WriteString("\nTimezone: ")
	b.WriteString(loc.String())
	b.WriteString("\nElapsed since last model-visible message: ")
	b.WriteString(elapsed)
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
