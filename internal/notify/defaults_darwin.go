//go:build darwin

package notify

// defaultNotifyCmd posts through AppleScript's notification center. Title and
// body are read from the environment via `system attribute` (Notify exports
// them as PYLON_TITLE/PYLON_BODY) rather than interpolated into the script
// text, so untrusted briefing content can never be parsed as AppleScript.
func defaultNotifyCmd() []string {
	return []string{"osascript", "-e",
		`display notification (system attribute "PYLON_BODY") with title (system attribute "PYLON_TITLE")`}
}
