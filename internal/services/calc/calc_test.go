package calc

import (
	"context"
	"math"
	"testing"

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

func TestFormatTR(t *testing.T) {
	cases := map[float64]string{
		42:        "42",
		-7:        "-7",
		42.857142: "42,86",
		43.2:      "43,20",
		1024:      "1024",
	}
	for in, want := range cases {
		if got := formatTR(in); got != want {
			t.Errorf("formatTR(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestExecute(t *testing.T) {
	c := New()
	out, err := c.Execute(context.Background(), ActionEval, map[string]string{"expr": "300/7"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "42,86 eder." {
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
