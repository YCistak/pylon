//go:build darwin

package notify

// defaultNotifyCmd posts through AppleScript's notification center.
func defaultNotifyCmd() []string {
	return []string{"osascript", "-e", `display notification "{body}" with title "{title}"`}
}
