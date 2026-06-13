// Package intent turns a user's transcribed text into a structured Command.
//
// It is two-tier: a local Router (this package, no API) resolves frequent
// commands by phrase + fuzzy matching, and only unresolved/novel input is meant
// to fall back to the Gemini engine. Keeping the common case local is what makes
// Pylon cheap to run.
package intent

// Action identifies what the user wants done. Values are namespaced so the
// Gemini fallback can return the same vocabulary.
type Action string

const (
	ActionUnknown    Action = "" // not resolved locally → defer to Gemini
	ActionLockScreen Action = "system.lock_screen"
	ActionMediaPlay  Action = "media.play"
	ActionMediaPause Action = "media.pause"
	ActionMediaNext  Action = "media.next"
	ActionMediaPrev  Action = "media.prev"
	ActionVolumeUp   Action = "media.volume_up"
	ActionVolumeDown Action = "media.volume_down"
	ActionMute       Action = "media.mute"

	// ActionRemindOnExit schedules a reminder fired when a process exits.
	// Args: "process" = process name, "content" = reminder text.
	ActionRemindOnExit Action = "task.remind_on_exit"
)

// Command is the structured result of interpreting a transcript.
type Command struct {
	Action     Action            `json:"action"`
	Args       map[string]string `json:"args,omitempty"`
	Confidence float64           `json:"confidence"` // 0..1; how sure the local router is
}

// Resolved reports whether the command was understood locally.
func (c Command) Resolved() bool { return c.Action != ActionUnknown }

func (c Command) arg(key string) string {
	if c.Args == nil {
		return ""
	}
	return c.Args[key]
}
