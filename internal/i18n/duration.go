package i18n

import "time"

// Duration renders a span the way it would be said out loud: "2 hours 5
// minutes", "2 saat 5 dakika", "2 часа 5 минут". Rounding to the minute is
// deliberate — seconds are noise in an answer about a day.
//
// It lives here rather than in the packages that speak it (work, sysmon) for
// the reason plural rules exist at all: "1 hour" and "2 hours" differ in
// English, not in Turkish, and in three ways in Russian. One implementation
// means one place to be right.
func Duration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	// Checked before rounding: half a minute would round up to "1 minute", which
	// overstates a session that barely happened.
	if d < time.Minute {
		return T("time.less_than_minute")
	}
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)

	switch {
	case h > 0 && m > 0:
		return T("time.join", N("time.hours", h), N("time.minutes", m))
	case h > 0:
		return N("time.hours", h)
	case m > 0:
		return N("time.minutes", m)
	default:
		return T("time.less_than_minute")
	}
}

// Uptime is Duration with a day component, for spans long enough that hours
// alone stop being readable ("3 days 4 hours").
func Uptime(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return T("time.join", N("time.days", days), N("time.hours", hours))
	case hours > 0:
		return T("time.join", N("time.hours", hours), N("time.minutes", mins))
	default:
		return N("time.minutes", mins)
	}
}
