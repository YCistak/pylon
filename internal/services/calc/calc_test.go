package calc

import (
	"context"
	"math"
	"testing"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

func TestEval(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"300/7", 300.0 / 7},
		{"12*8+5", 101},
		{"2^10", 1024},
		{"2^3^2", 512}, // right-associative: 2^(3^2)
		{"-5+3", -2},
		{"(1+2)*3", 9},
		{"10 % 3", 1},
		{"240*18/100", 43.2},
		{"3.5 * 2", 7},
		{"-(4)^2", -16}, // unary minus binds looser than power
		{"  7  -  2 ", 5},
	}
	for _, c := range cases {
		got, err := Eval(c.expr)
		if err != nil {
			t.Errorf("Eval(%q) error: %v", c.expr, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalErrors(t *testing.T) {
	for _, expr := range []string{"1/0", "5 %0", "", "2+", "(1+2", "3 4", "abc", "2^"} {
		if _, err := Eval(expr); err == nil {
			t.Errorf("Eval(%q): expected error", expr)
		}
	}
}

// A result is punctuated by the language it is spoken in: the decimal point is a
// comma in Turkish and a point in English, and a whole number carries neither.
func TestFormatResult(t *testing.T) {
	perLanguage := map[string]map[float64]string{
		"en": {42: "42", -7: "-7", 42.857142: "42.86", 43.2: "43.20", 1024: "1024"},
		"tr": {42: "42", -7: "-7", 42.857142: "42,86", 43.2: "43,20", 1024: "1024"},
	}
	for lang, cases := range perLanguage {
		i18n.SetLanguage(lang)
		for in, want := range cases {
			if got := formatResult(in); got != want {
				t.Errorf("formatResult(%v) in %s = %q, want %q", in, lang, got, want)
			}
		}
	}
	i18n.SetLanguage(i18n.Default)
}

// The result sentence used to be built with fmt.Sprintf("%s eder.", ...), so
// asking in English got a Turkish answer. Nothing about arithmetic is
// language-specific, which is exactly why the wording was easy to forget.
func TestExecuteAnswersInTheActiveLanguage(t *testing.T) {
	c := New()
	for lang, want := range map[string]string{
		"en": "That's 84.",
		"tr": "84 eder.",
		"de": "Das sind 84.",
	} {
		i18n.SetLanguage(lang)
		out, err := c.Execute(context.Background(), ActionEval, map[string]string{"expr": "12*7"})
		if err != nil {
			t.Fatalf("Execute in %s: %v", lang, err)
		}
		if out != want {
			t.Errorf("Execute in %s = %q, want %q", lang, out, want)
		}
	}
	i18n.SetLanguage(i18n.Default)
}

func TestExecute(t *testing.T) {
	c := New()
	out, err := c.Execute(context.Background(), ActionEval, map[string]string{"expr": "300/7"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "That's 42.86." {
		t.Fatalf("Execute output %q", out)
	}

	// Empty expr is a usage error.
	if _, err := c.Execute(context.Background(), ActionEval, map[string]string{"expr": ""}); err == nil {
		t.Error("expected error for empty expr")
	}

	// A malformed expression yields a graceful spoken message, not an error.
	out, err = c.Execute(context.Background(), ActionEval, map[string]string{"expr": "3 4 5"})
	if err != nil {
		t.Fatalf("malformed Execute error: %v", err)
	}
	if out != "I couldn't work that out." {
		t.Fatalf("malformed output %q", out)
	}

	if _, err := c.Execute(context.Background(), "calc.bogus", nil); err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestActionsSpec(t *testing.T) {
	specs := New().Actions()
	if len(specs) != 1 || specs[0].Name != ActionEval {
		t.Fatalf("unexpected specs: %+v", specs)
	}
	if len(specs[0].Args) != 1 || specs[0].Args[0] != "expr" {
		t.Fatalf("expected expr arg, got %+v", specs[0].Args)
	}
	var _ intent.Action = ActionEval
}
