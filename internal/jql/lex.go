package jql

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tNumber
	tDuration
	tLParen
	tRParen
	tLBracket
	tRBracket
	tComma
	tEq
	tNeq
	tTilde
	tNtilde
	tGt
	tGte
	tLt
	tLte
)

type token struct {
	kind tokKind
	val  string
	pos  int
}

func lex(s string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			i += w
			continue
		}
		pos := i
		if i+1 < len(s) {
			two := s[i : i+2]
			switch two {
			case "!=":
				toks = append(toks, token{tNeq, two, pos})
				i += 2
				continue
			case "!~":
				toks = append(toks, token{tNtilde, two, pos})
				i += 2
				continue
			case ">=":
				toks = append(toks, token{tGte, two, pos})
				i += 2
				continue
			case "<=":
				toks = append(toks, token{tLte, two, pos})
				i += 2
				continue
			}
		}
		switch r {
		case '(':
			toks = append(toks, token{tLParen, "(", pos})
			i++
		case ')':
			toks = append(toks, token{tRParen, ")", pos})
			i++
		case '[':
			toks = append(toks, token{tLBracket, "[", pos})
			i++
		case ']':
			toks = append(toks, token{tRBracket, "]", pos})
			i++
		case ',':
			toks = append(toks, token{tComma, ",", pos})
			i++
		case '=':
			toks = append(toks, token{tEq, "=", pos})
			i++
		case '~':
			toks = append(toks, token{tTilde, "~", pos})
			i++
		case '>':
			toks = append(toks, token{tGt, ">", pos})
			i++
		case '<':
			toks = append(toks, token{tLt, "<", pos})
			i++
		case '"', '\'':
			tok, next, err := lexString(s, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, tok)
			i = next
		default:
			if r == '-' && i+1 < len(s) && unicode.IsDigit(rune(s[i+1])) {
				tok, next := lexDurationOrNumber(s, i)
				toks = append(toks, tok)
				i = next
				continue
			}
			if unicode.IsDigit(r) {
				tok, next := lexDurationOrNumber(s, i)
				toks = append(toks, tok)
				i = next
				continue
			}
			if isIdentStart(r) {
				j := i + w
				for j < len(s) {
					rj, wj := utf8.DecodeRuneInString(s[j:])
					if !isIdentCont(rj) {
						break
					}
					j += wj
				}
				toks = append(toks, token{tIdent, s[i:j], pos})
				i = j
				continue
			}
			return nil, fmt.Errorf("unexpected %q at %d", string(r), pos)
		}
	}
	toks = append(toks, token{kind: tEOF, pos: len(s)})
	return toks, nil
}

func lexString(s string, i int) (token, int, error) {
	quote := s[i]
	pos := i
	i++
	var b strings.Builder
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if c == quote {
			return token{tString, b.String(), pos}, i + 1, nil
		}
		b.WriteByte(c)
		i++
	}
	return token{}, 0, fmt.Errorf("unclosed string at %d", pos)
}

func lexDurationOrNumber(s string, i int) (token, int) {
	pos := i
	if s[i] == '-' {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) {
		u := s[i]
		if u == 'd' || u == 'D' || u == 'w' || u == 'W' ||
			u == 'm' || u == 'M' || u == 'y' || u == 'Y' ||
			u == 'h' || u == 'H' || u == 's' || u == 'S' {
			return token{tDuration, s[pos : i+1], pos}, i + 1
		}
	}
	return token{tNumber, s[pos:i], pos}, i
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentCont(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
