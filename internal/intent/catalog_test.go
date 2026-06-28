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
	cmd := decodeCommandFields("calendar.add_event", "", "Diş hekimi", "", "2026-06-15T15:00:00+03:00")
	if cmd.arg("datetime") != "2026-06-15T15:00:00+03:00" {
		t.Fatalf("datetime = %q", cmd.arg("datetime"))
	}
	if cmd.arg("content") != "Diş hekimi" {
		t.Fatalf("content = %q", cmd.arg("content"))
	}
}
