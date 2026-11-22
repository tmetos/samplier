// Package units implements concentration and length unit conversion using the
// factor/delta model stored in unit_conversion:
//
//	converted = value * conversion_factor + delta
//
// Non-linear conversions (for example temperature) may supply a formula.
package units

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrNoConversion is returned when no path exists between two units.
var ErrNoConversion = errors.New("no unit conversion available")

// ErrUnknownFormula is returned when a formula references an unknown token.
var ErrUnknownFormula = errors.New("unknown token in conversion formula")

// Conversion is a single conversion rule from one unit to another.
type Conversion struct {
	ReportedUnit     string
	TargetUnit       string
	ConversionFactor float64
	Delta            float64
	// Formula, when non-empty, overrides the factor/delta calculation.
	// Supported tokens: {original_value}, {conversion_factor}, {delta},
	// {a}, {b}, {c}, {d}, {e}. Operators: + - * / and parentheses.
	Formula       string
	A, B, C, D, E float64
}

// Table holds conversion rules and answers unit-to-unit queries.
type Table struct {
	rules map[string][]Conversion
}

// NewTable builds a conversion table from rules.
func NewTable(rules []Conversion) *Table {
	t := &Table{rules: make(map[string][]Conversion, len(rules))}
	for _, r := range rules {
		key := normalize(r.ReportedUnit)
		t.rules[key] = append(t.rules[key], r)
	}
	return t
}

// Convert converts value from one unit to another. Identity conversions
// (same unit, case-insensitive) return the value unchanged. A direct rule is
// preferred, otherwise a single intermediate unit is tried.
func (t *Table) Convert(value float64, from, to string) (float64, error) {
	if normalize(from) == normalize(to) {
		return value, nil
	}

	if direct, ok := t.lookup(from, to); ok {
		return apply(direct, value)
	}

	// One-level bridge: from -> mid -> to.
	for _, first := range t.rules[normalize(from)] {
		second, ok := t.lookup(first.TargetUnit, to)
		if !ok {
			continue
		}
		mid, err := apply(first, value)
		if err != nil {
			return 0, err
		}
		return apply(second, mid)
	}

	return 0, fmt.Errorf("%w: %s -> %s", ErrNoConversion, from, to)
}

// CanConvert reports whether a conversion path exists.
func (t *Table) CanConvert(from, to string) bool {
	if normalize(from) == normalize(to) {
		return true
	}
	if _, ok := t.lookup(from, to); ok {
		return true
	}
	for _, first := range t.rules[normalize(from)] {
		if _, ok := t.lookup(first.TargetUnit, to); ok {
			return true
		}
	}
	return false
}

func (t *Table) lookup(from, to string) (Conversion, bool) {
	for _, r := range t.rules[normalize(from)] {
		if normalize(r.TargetUnit) == normalize(to) {
			return r, true
		}
	}
	return Conversion{}, false
}

func apply(c Conversion, value float64) (float64, error) {
	formula := strings.TrimSpace(c.Formula)
	if formula == "" {
		return value*c.ConversionFactor + c.Delta, nil
	}
	return evalFormula(formula, map[string]float64{
		"original_value":    value,
		"conversion_factor": c.ConversionFactor,
		"delta":             c.Delta,
		"a":                 c.A,
		"b":                 c.B,
		"c":                 c.C,
		"d":                 c.D,
		"e":                 c.E,
	})
}

func normalize(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// --- formula evaluator -------------------------------------------------------
// Recursive-descent parser supporting + - * /, parentheses, unary minus,
// numeric literals and {token} references.

func evalFormula(formula string, vars map[string]float64) (float64, error) {
	p := &parser{input: formula, vars: vars}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("%w: %q", ErrUnknownFormula, p.input[p.pos:])
	}
	return v, nil
}

type parser struct {
	input string
	pos   int
	vars  map[string]float64
}

func (p *parser) skipSpace() {
	for p.pos < len(p.input) {
		switch p.input[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) peek() byte {
	p.skipSpace()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left += right
		case '-':
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			left *= right
		case '/':
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero in formula")
			}
			left /= right
		default:
			return left, nil
		}
	}
}

func (p *parser) parseFactor() (float64, error) {
	switch c := p.peek(); {
	case c == '-':
		p.pos++
		v, err := p.parseFactor()
		return -v, err
	case c == '+':
		p.pos++
		return p.parseFactor()
	case c == '(':
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	case c == '{':
		return p.parseVar()
	case c >= '0' && c <= '9' || c == '.':
		return p.parseNumber()
	default:
		return 0, fmt.Errorf("%w: unexpected %q", ErrUnknownFormula, string(c))
	}
}

func (p *parser) parseVar() (float64, error) {
	p.pos++ // consume '{'
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '}' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("missing closing brace")
	}
	name := strings.ToLower(strings.TrimSpace(p.input[start:p.pos]))
	p.pos++ // consume '}'
	v, ok := p.vars[name]
	if !ok {
		return 0, fmt.Errorf("%w: {%s}", ErrUnknownFormula, name)
	}
	return v, nil
}

func (p *parser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c >= '0' && c <= '9' || c == '.' || c == 'e' || c == 'E' ||
			(c == '-' && p.pos > start && (p.input[p.pos-1] == 'e' || p.input[p.pos-1] == 'E')) {
			p.pos++
			continue
		}
		break
	}
	v, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", p.input[start:p.pos])
	}
	return v, nil
}
