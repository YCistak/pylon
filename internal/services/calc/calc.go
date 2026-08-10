// Package calc is Pylon's arithmetic service: it evaluates a math expression and
// speaks the result ("300 bölü 7 kaç eder"). The intent engine turns spoken
// Turkish math into a plain expression (bölü→/, çarpı→*, artı→+, eksi→-,
// üzeri→^) and passes it as the `expr` arg; this package parses and computes it
// with a small dependency-free recursive-descent evaluator (no eval, no cgo).
package calc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"context"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// ActionEval evaluates the `expr` arithmetic expression.
const ActionEval intent.Action = "calc.eval"

// Calc is the arithmetic Service. It needs no configuration, so it is always
// registered.
type Calc struct{}

// New builds the service.
func New() *Calc { return &Calc{} }

func (c *Calc) Name() string { return "calc" }

func (c *Calc) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionEval,
			Args: []string{"expr"},
			Desc: `"calc.eval": evaluate an arithmetic expression. Arg "expr" is a plain math expression using digits and + - * / % ^ ( ) and a . decimal point. Convert spoken math to it: bölü→/, çarpı/kere→*, artı→+, eksi→-, üzeri/üssü→^, yüzde X→(X/100). Examples: "300 bölü 7"→expr:"300/7"; "12 kere 8 artı 5"→expr:"12*8+5"; "2 üzeri 10"→expr:"2^10"; "240'ın yüzde 18'i"→expr:"240*18/100".`,
		},
	}
}

func (c *Calc) Execute(_ context.Context, action intent.Action, args map[string]string) (string, error) {
	switch action {
	case ActionEval:
		expr := strings.TrimSpace(args["expr"])
		if expr == "" {
			return "", errors.New("calc: empty expression")
		}
		v, err := Eval(expr)
		if err != nil {
			return i18n.T("calc.unparsable"), nil
		}
		return fmt.Sprintf("%s eder.", formatTR(v)), nil
	default:
		return "", fmt.Errorf("calc: bilinmeyen aksiyon %q", action)
	}
}

// formatTR renders a result for speech in Turkish: integers without decimals,
// otherwise up to two decimals with a comma separator ("42,86").
func formatTR(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	return strings.Replace(s, ".", ",", 1)
}

// Eval parses and evaluates an arithmetic expression. Supported: + - * / %
// (modulo), ^ (power, right-associative), unary minus, parentheses, and decimal
// numbers. Division or modulo by zero is an error.
func Eval(expr string) (float64, error) {
	p := &parser{src: expr}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("beklenmeyen karakter: %q", p.src[p.pos:])
	}
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, errors.New("undefined result")
	}
	return v, nil
}

// parser is a tiny recursive-descent evaluator over src.
//
//	expr   = term  (('+' | '-') term)*
//	term   = unary (('*' | '/' | '%') unary)*
//	unary  = ('-' | '+') unary | power
//	power  = primary ('^' unary)?          // right-associative; binds tighter than
//	                                       // unary minus, so -4^2 = -(4^2) = -16
//	primary= number | '(' expr ')'
type parser struct {
	src string
	pos int
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && p.src[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return v, nil
		}
		switch p.src[p.pos] {
		case '+':
			p.pos++
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v += r
		case '-':
			p.pos++
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v -= r
		default:
			return v, nil
		}
	}
}

func (p *parser) parseTerm() (float64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return v, nil
		}
		switch p.src[p.pos] {
		case '*':
			p.pos++
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			v *= r
		case '/':
			p.pos++
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, errors.New("division by zero")
			}
			v /= r
		case '%':
			p.pos++
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, errors.New("division by zero")
			}
			v = math.Mod(v, r)
		default:
			return v, nil
		}
	}
}

func (p *parser) parseUnary() (float64, error) {
	p.skipSpace()
	if p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '-':
			p.pos++
			v, err := p.parseUnary()
			return -v, err
		case '+':
			p.pos++
			return p.parseUnary()
		}
	}
	return p.parsePower()
}

func (p *parser) parsePower() (float64, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '^' {
		p.pos++
		exp, err := p.parseUnary() // right-associative; allows 2^-1
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return base, nil
}

func (p *parser) parsePrimary() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return 0, errors.New("eksik ifade")
	}
	if p.src[p.pos] == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return 0, errors.New("kapanmayan parantez")
		}
		p.pos++
		return v, nil
	}
	return p.parseNumber()
}

func (p *parser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.src) {
		ch := rune(p.src[p.pos])
		if unicode.IsDigit(ch) || ch == '.' {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected a number: %q", p.src[p.pos:])
	}
	return strconv.ParseFloat(p.src[start:p.pos], 64)
}
