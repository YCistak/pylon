// Package intent turns a user's transcribed text into a structured Command.
//
// It is two-tier: a local Router (this package, no API) resolves frequent
// commands by phrase + fuzzy matching, and only unresolved/novel input is meant
// to fall back to the Gemini engine. Keeping the common case local is what makes
// Pylon cheap to run.
package intent

import "sort"

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

	// ActionNowPlaying asks what is playing on this machine. Named with the
	// other media actions and not after any one player, because the answer
	// comes from whatever is running — the local router resolves it, so it
	// costs no model call.
	ActionNowPlaying Action = "media.now_playing"

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

// ActionSpec describes one action the LLM may choose from. Desc, when set, is a
// rule injected into the system prompt; Args names the arg fields the action
// populates. Built-in actions plus service-contributed ones form the catalog the
// LLM schema and prompt are built from.
type ActionSpec struct {
	Name Action
	Desc string
	Args []string
}

// builtinActions is Pylon's core action vocabulary. The media/lock/close actions
// are declared and executed by the system service (internal/services/system),
// which is always registered; here we keep only remind and chat, which carry
// prompt rules and are handled inside the daemon rather than by a service.
func builtinActions() []ActionSpec {
	return []ActionSpec{
		{
			Name: ActionRemindOnExit,
			Args: []string{"process", "content"},
			Desc: `For "task.remind_on_exit": "process" is the app's canonical executable name as a single lowercase word (e.g. "code" for VSCode, "steam" for Steam, "cs2" for Counter-Strike 2); "content" is the reminder text as a short imperative WITHOUT the trigger clause. Never leave "content" empty for this action.
  Example: "steam kapanınca ödevimi yapmayı unutma de" -> {"action":"task.remind_on_exit","process":"steam","content":"ödevini yapmayı unutma"}`,
		},
		{
			Name: ActionChat,
			Args: []string{"reply"},
			Desc: `For casual conversation, questions you can answer directly, or any request with no matching action, use "chat" and put the answer in "reply". Keep "reply" SHORT — one or two sentences, calm and composed (think JARVIS), no filler/preamble/AI-disclaimers; it is spoken aloud. If the message is unintelligible or a speech-to-text error (gibberish), reply briefly "Anlamadım, tekrar eder misin?".`,
		},
	}
}

// actionCatalog is the live catalog: built-ins, optionally extended by services.
var actionCatalog = builtinActions()

// SetActions rebuilds the catalog as the built-ins plus the given extra (service)
// specs. Call once at startup, before the LLM chain handles input.
func SetActions(extra ...ActionSpec) {
	actionCatalog = append(builtinActions(), extra...)
}

func catalog() []ActionSpec { return actionCatalog }

// coreArgs are in the schema whatever the catalog holds: the built-in actions
// populate process/content/reply, and "datetime" carries a resolved absolute
// time for any action that needs one — the prompt asks for it as a general rule
// rather than per action, so no ActionSpec declares it.
var coreArgs = []string{"process", "content", "reply", "datetime"}

// argFields is every argument name the model may return, sorted.
//
// The schema used to hard-code the core four. Any action declaring a different
// argument — calc.eval's "expr", docker's "container", exchange's "base"/"quote"
// — was therefore chosen correctly and then handed nothing: the model had no
// field to put the value in, so the service received an empty string and
// answered "hangi para birimini soruyorsun?" to a perfectly clear question.
// Deriving the fields from the catalog is what keeps that from happening again
// every time a service is added.
func argFields() []string {
	seen := make(map[string]bool, len(coreArgs))
	out := make([]string, 0, len(coreArgs))
	add := func(name string) {
		// "action" is the command itself, not one of its arguments.
		if name == "" || name == "action" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, a := range coreArgs {
		add(a)
	}
	for _, s := range catalog() {
		for _, a := range s.Args {
			add(a)
		}
	}
	sort.Strings(out)
	return out
}
