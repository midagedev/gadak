package jql

import (
	"fmt"
	"strings"
)

type nodeKind int

const (
	nAnd nodeKind = iota
	nOr
	nNot
	nClause
)

type node struct {
	kind   nodeKind
	left   *node
	right  *node // and/or
	clause *clause
}

type clauseOp int

const (
	opEq clauseOp = iota
	opNeq
	opTilde
	opNtilde
	opGt
	opGte
	opLt
	opLte
	opIn
	opNotIn
	opIsEmpty
	opIsNotEmpty
)

func (op clauseOp) String() string {
	switch op {
	case opEq:
		return "="
	case opNeq:
		return "!="
	case opTilde:
		return "~"
	case opNtilde:
		return "!~"
	case opGt:
		return ">"
	case opGte:
		return ">="
	case opLt:
		return "<"
	case opLte:
		return "<="
	case opIn:
		return "in"
	case opNotIn:
		return "not in"
	case opIsEmpty:
		return "is EMPTY"
	case opIsNotEmpty:
		return "is not EMPTY"
	default:
		return "?"
	}
}

type clause struct {
	field  string
	op     clauseOp
	values []value
	raw    string
}

type valKind int

const (
	valString valKind = iota
	valIdent
	valNumber
	valDuration
	valFunc
)

type value struct {
	kind     valKind
	raw      string
	funcName string
	args     []value
}

func (v value) text() string {
	switch v.kind {
	case valFunc:
		parts := make([]string, len(v.args))
		for i, a := range v.args {
			parts[i] = a.text()
		}
		return v.funcName + "(" + strings.Join(parts, ", ") + ")"
	case valString:
		return `"` + v.raw + `"`
	default:
		return v.raw
	}
}

func (c *clause) render() string {
	if c.raw != "" {
		return c.raw
	}
	switch c.op {
	case opIsEmpty, opIsNotEmpty:
		return c.field + " " + c.op.String()
	case opIn, opNotIn:
		parts := make([]string, len(c.values))
		for i, v := range c.values {
			parts[i] = v.text()
		}
		return c.field + " " + c.op.String() + " (" + strings.Join(parts, ", ") + ")"
	default:
		if len(c.values) == 0 {
			return c.field + " " + c.op.String()
		}
		return c.field + " " + c.op.String() + " " + c.values[0].text()
	}
}

type order struct {
	field string
	dir   string // asc|desc|""
}

type parser struct {
	toks []token
	i    int
	err  error
}

func parse(s string) (*node, []order, error) {
	toks, err := lex(s)
	if err != nil {
		return nil, nil, err
	}
	p := &parser{toks: toks}
	n := p.parseOr()
	if p.err != nil {
		return nil, nil, p.err
	}
	var orders []order
	if p.kw("order") {
		p.next()
		if !p.kw("by") {
			return nil, nil, p.fail("expected BY after ORDER")
		}
		p.next()
		for {
			o := order{field: p.parseField()}
			if p.err != nil {
				return nil, nil, p.err
			}
			if p.kw("asc") {
				o.dir = "asc"
				p.next()
			} else if p.kw("desc") {
				o.dir = "desc"
				p.next()
			}
			orders = append(orders, o)
			if p.peek().kind != tComma {
				break
			}
			p.next()
		}
	}
	if p.peek().kind != tEOF {
		return nil, nil, p.fail("unexpected %q", p.peek().val)
	}
	return n, orders, nil
}

func (p *parser) peek() token {
	if p.i >= len(p.toks) {
		return token{kind: tEOF}
	}
	return p.toks[p.i]
}

func (p *parser) next() token {
	t := p.peek()
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func (p *parser) kw(s string) bool {
	t := p.peek()
	return t.kind == tIdent && strings.EqualFold(t.val, s)
}

func (p *parser) fail(format string, a ...any) error {
	if p.err != nil {
		return p.err
	}
	msg := fmt.Sprintf(format, a...)
	p.err = fmt.Errorf("%s at %d", msg, p.peek().pos)
	return p.err
}

func (p *parser) parseOr() *node {
	left := p.parseAnd()
	for p.err == nil && p.kw("or") {
		p.next()
		right := p.parseAnd()
		left = &node{kind: nOr, left: left, right: right}
	}
	return left
}

func (p *parser) parseAnd() *node {
	left := p.parseNot()
	for p.err == nil {
		if p.kw("and") {
			p.next()
			left = &node{kind: nAnd, left: left, right: p.parseNot()}
			continue
		}
		if p.startsClause() {
			left = &node{kind: nAnd, left: left, right: p.parseNot()}
			continue
		}
		break
	}
	return left
}

func (p *parser) startsClause() bool {
	t := p.peek()
	if t.kind == tLParen {
		return true
	}
	if t.kind != tIdent && t.kind != tString {
		return false
	}
	if t.kind == tIdent {
		switch strings.ToLower(t.val) {
		case "or", "and", "order", "asc", "desc":
			return false
		}
	}
	return true
}

func (p *parser) parseNot() *node {
	if p.kw("not") {
		// `NOT IN` belongs to a clause, not a prefix NOT. Prefix NOT is
		// `NOT (…)` or `NOT field = …`.
		p.next()
		return &node{kind: nNot, left: p.parseNot()}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() *node {
	if p.peek().kind == tLParen {
		p.next()
		n := p.parseOr()
		if p.peek().kind != tRParen {
			p.fail("expected )")
			return n
		}
		p.next()
		return n
	}
	c := p.parseClause()
	return &node{kind: nClause, clause: c}
}

func (p *parser) parseField() string {
	t := p.peek()
	if t.kind == tString {
		p.next()
		return t.val
	}
	if t.kind != tIdent {
		p.fail("expected field name")
		return ""
	}
	name := p.next().val
	if p.peek().kind == tLBracket {
		p.next()
		if p.peek().kind != tNumber && p.peek().kind != tIdent {
			p.fail("expected field id inside []")
			return name
		}
		id := p.next().val
		if p.peek().kind != tRBracket {
			p.fail("expected ]")
			return name
		}
		p.next()
		return name + "[" + id + "]"
	}
	return name
}

func (p *parser) parseClause() *clause {
	field := p.parseField()
	c := &clause{field: field}

	if p.kw("not") {
		p.next()
		if !p.kw("in") {
			p.fail("expected IN after NOT")
			return c
		}
		p.next()
		c.op = opNotIn
		c.values = p.parseInValues()
		c.raw = c.render()
		return c
	}
	if p.kw("in") {
		p.next()
		c.op = opIn
		c.values = p.parseInValues()
		c.raw = c.render()
		return c
	}
	if p.kw("is") {
		p.next()
		not := false
		if p.kw("not") {
			not = true
			p.next()
		}
		if !p.kw("empty") {
			p.fail("expected EMPTY after IS")
			return c
		}
		p.next()
		if not {
			c.op = opIsNotEmpty
		} else {
			c.op = opIsEmpty
		}
		c.raw = c.render()
		return c
	}

	switch p.peek().kind {
	case tEq:
		c.op = opEq
	case tNeq:
		c.op = opNeq
	case tTilde:
		c.op = opTilde
	case tNtilde:
		c.op = opNtilde
	case tGt:
		c.op = opGt
	case tGte:
		c.op = opGte
	case tLt:
		c.op = opLt
	case tLte:
		c.op = opLte
	default:
		p.fail("expected operator after %s", field)
		return c
	}
	p.next()
	c.values = []value{p.parseValue()}
	c.raw = c.render()
	return c
}

func (p *parser) parseInValues() []value {
	if p.peek().kind == tIdent && p.i+1 < len(p.toks) && p.toks[p.i+1].kind == tLParen {
		return []value{p.parseValue()}
	}
	return p.parseList()
}

func (p *parser) parseList() []value {
	if p.peek().kind != tLParen {
		p.fail("expected (")
		return nil
	}
	p.next()
	var out []value
	if p.peek().kind == tRParen {
		p.next()
		return out
	}
	for {
		out = append(out, p.parseValue())
		if p.peek().kind == tComma {
			p.next()
			continue
		}
		break
	}
	if p.peek().kind != tRParen {
		p.fail("expected )")
		return out
	}
	p.next()
	return out
}

func (p *parser) parseValue() value {
	t := p.peek()
	switch t.kind {
	case tString:
		p.next()
		return value{kind: valString, raw: t.val}
	case tNumber:
		p.next()
		return value{kind: valNumber, raw: t.val}
	case tDuration:
		p.next()
		return value{kind: valDuration, raw: t.val}
	case tIdent:
		p.next()
		if p.peek().kind == tLParen {
			p.next()
			var args []value
			if p.peek().kind != tRParen {
				for {
					args = append(args, p.parseValue())
					if p.peek().kind == tComma {
						p.next()
						continue
					}
					break
				}
			}
			if p.peek().kind != tRParen {
				p.fail("expected ) after function")
			} else {
				p.next()
			}
			return value{kind: valFunc, funcName: t.val, args: args}
		}
		return value{kind: valIdent, raw: t.val}
	default:
		p.fail("expected value")
		return value{}
	}
}
