package intent

import (
	"slices"
	"strings"
	"testing"
)

func TestSetActionsExtendsCatalog(t *testing.T) {
	defer SetActions() // reset to built-ins so other tests see the default catalog

	SetActions(ActionSpec{Name: "calendar.add_event", Desc: "add a calendar event"})

	all := allActions()
	if !slices.Contains(all, "calendar.add_event") {
		t.Fatalf("catalog missing service action: %v", all)
	}
	if !slices.Contains(all, string(ActionChat)) {
		t.Fatalf("built-ins dropped after SetActions: %v", all)
	}
	if !strings.Contains(systemPrompt(""), "add a calendar event") {
		t.Fatal("service action desc not injected into system prompt")
	}
}

func TestCommandSchemaHasDatetime(t *testing.T) {
	s := commandSchema()
	if _, ok := s.Properties["datetime"]; !ok {
		t.Fatal("schema missing datetime property")
	}
	if !slices.Contains(s.Required, "datetime") {
		t.Fatal("datetime not required")
	}
}

func TestDecodeCommandFieldsDatetime(t *testing.T) {
	cmd := decodeCommandArgs(map[string]string{
		"action":   "calendar.add_event",
		"content":  "Diş hekimi",
		"datetime": "2026-06-15T15:00:00+03:00",
	})
	if cmd.arg("datetime") != "2026-06-15T15:00:00+03:00" {
		t.Fatalf("datetime = %q", cmd.arg("datetime"))
	}
	if cmd.arg("content") != "Diş hekimi" {
		t.Fatalf("content = %q", cmd.arg("content"))
	}
}

// A service's own argument name must reach both the schema and the prompt, or
// the model can pick the action and has nowhere to put the value — how
// "12 çarpı 7 kaç eder" used to arrive at calc.eval with an empty expr.
func TestServiceArgReachesSchemaAndPrompt(t *testing.T) {
	defer SetActions() // restore the built-in catalog for the other tests

	SetActions(ActionSpec{
		Name: "calc.eval",
		Args: []string{"expr"},
		Desc: `"calc.eval": evaluate an arithmetic expression in "expr".`,
	})

	if _, ok := commandSchema().Properties["expr"]; !ok {
		t.Error("gemini schema has no expr property")
	}
	if !slices.Contains(commandSchema().Required, "expr") {
		t.Error("expr not required in the gemini schema")
	}
	props, _ := jsonSchemaCommand()["properties"].(map[string]any)
	if _, ok := props["expr"]; !ok {
		t.Error("json schema has no expr property")
	}
	if !strings.Contains(systemPrompt(""), `"expr"`) {
		t.Error("the prompt never names expr, so the model is not told to fill it")
	}

	// The core four survive a catalog that does not mention them.
	for _, f := range []string{"process", "content", "reply", "datetime"} {
		if _, ok := commandSchema().Properties[f]; !ok {
			t.Errorf("core field %q dropped from the schema", f)
		}
	}
}

// Whatever the model returns under a service's arg name must arrive verbatim.
func TestDecodeCommandArgsKeepsServiceArgs(t *testing.T) {
	cmd, err := decodeCommandJSON([]byte(`{
		"action": "docker.logs", "container": "freshrss", "lines": 50,
		"process": "", "content": "", "reply": "", "datetime": ""
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != "docker.logs" {
		t.Fatalf("action = %q", cmd.Action)
	}
	if cmd.arg("container") != "freshrss" {
		t.Fatalf("container = %q", cmd.arg("container"))
	}
	// A number where the schema asked for a string should still not lose the arg.
	if cmd.arg("lines") != "50" {
		t.Fatalf("lines = %q", cmd.arg("lines"))
	}
	// Empty strings are dropped, as they always have been.
	if cmd.arg("reply") != "" {
		t.Fatalf("empty reply kept: %q", cmd.arg("reply"))
	}
}
