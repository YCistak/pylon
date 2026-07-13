//go:build linux

package notify

// defaultNotifyCmd posts through notify-send (libnotify), served by dunst, mako,
// GNOME/KDE, etc. `-a Pylon` sets the app name; title and body are separate argv
// entries so they need no shell escaping.
func defaultNotifyCmd() []string {
	return []string{"notify-send", "-a", "Pylon", "{title}", "{body}"}
}
