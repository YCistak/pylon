//go:build windows

package notify

// defaultNotifyCmd has no built-in Windows default yet (toast requires a helper);
// set notify.cmd in config to enable. Empty makes Notify a no-op error.
func defaultNotifyCmd() []string {
	return nil
}
