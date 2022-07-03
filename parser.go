//go:generate stringer -type stateL1
//go:generate stringer -type stateL2
package cofunc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/cofunclabs/cofunc/pkg/is"
)

func ParseAST(rd io.Reader) (*AST, error) {
	ast := newAST()
	num := 0
	scanner := bufio.NewScanner(rd)
	for {
		if !scanner.Scan() {
			break
		}
		num += 1
		err := scanToken(ast, scanner.Text(), num)
		if err != nil {
			return nil, err
		}
	}

	return ast, ast.Foreach(func(b *Block) error {
		if err := doBlockHeader(b); err != nil {
			return err
		}

		if err := doBlockBody(b); err != nil {
			return err
		}
		return nil
	})
}

func doBlockBody(b *Block) error {
	if b.bbody == nil {
		return nil
	}
	lines := b.bbody.List()
	for _, l := range lines {
		// handle tokens
		for _, t := range l.tokens {
			t.setblock(b)
			if err := t.extractVar(); err != nil {
				return err
			}
			if err := t.validate(); err != nil {
				return err
			}
		}

		if err := buildVarGraph(b, l); err != nil {
			return err
		}
	}
	return nil
}

func buildVarGraph(b *Block, stm *Statement) error {
	if stm.desc != "var" {
		return nil
	}
	name := stm.tokens[0].String()
	v := &_var{
		segments: []struct {
			str   string
			isvar bool
		}{},
		child: []*_var{},
	}
	if len(stm.tokens) == 2 {
		vt := stm.tokens[1]
		if !vt.HasVar() {
			v.v = vt.String()
			v.cached = true
		} else {
			v.segments = vt.Segments()
			for _, seg := range v.segments {
				if !seg.isvar {
					continue
				}
				vname := seg.str
				chld, ok := b.GetVar(vname)
				if ok {
					v.child = append(v.child, chld)
				}
			}
		}
	}
	if err := b.PutVar(name, v); err != nil {
		return err
	}
	return nil
}

func doBlockHeader(b *Block) error {
	ts := []*Token{
		&b.kind,
		&b.target,
		&b.operator,
		&b.typevalue,
	}
	for _, t := range ts {
		t.setblock(b)
		if err := t.extractVar(); err != nil {
			return err
		}
		if err := t.validate(); err != nil {
			return err
		}
	}
	return nil
}

const (
	_l1_global stateL1 = iota
	_l1_keyword
	_l1_load_started
	_l1_run_started
	_l1_run_body_started
	_l1_run_body_inside
	_l1_fn_started
	_l1_fn_body_started
	_l1_fn_body_inside
	_l1_args_started
	_l1_args_body_started
	_l1_args_body_inside
	_l1_var_started
)

const (
	_l2_unknow stateL2 = iota
	_l2_multilines_started
	_l2_word_started
	_l2_kind_started
	_l2_kind_done
	_l2_target_started
	_l2_target_done
	_l2_operator_started
	_l2_operator_done
	_l2_typevalue_started
)

func scanToken(ast *AST, line string, linenum int) error {
	block := ast.parsing

	var start int

	finiteAutomata := func(last int, current rune, newline string) error {
		switch ast.phase() {
		case _l1_global:
			// skip
			if is.SpaceOrEOL(current) {
				break
			}
			// transfer
			if is.Word(current) {
				start = last
				ast.transfer(_l1_keyword)
				break
			}
			// error
			return errors.New("contain invalid character: " + newline)
		case _l1_keyword:
			// keep
			if is.Word(current) {
				break
			}
			// transfer
			if is.Space(current) || is.LeftBracket(current) {
				var body bbody = nil
				word := newline[start:last]
				switch word {
				case "load":
					if is.LeftBracket(current) {
						return errors.New("contain invalid character: " + newline)
					}
					ast.transfer(_l1_load_started)
				case "fn":
					if is.LeftBracket(current) {
						return errors.New("contain invalid character: " + newline)
					}
					ast.transfer(_l1_fn_started)
				case "run":
					if is.LeftBracket(current) {
						body = &FList{etype: _functionname_t}
						ast.transfer(_l1_run_body_started)
					} else {
						ast.transfer(_l1_run_started)
					}
				case "var":
					if is.LeftBracket(current) {
						return errors.New("contain invalid character: " + newline)
					}
					ast.transfer(_l1_var_started)
					block.state = _l2_kind_done
					block.bbody.Append(newstm("var"))
					// var is not a block, so return
					return nil
				default:
					return errors.New("invalid block define: " + word)
				}
				newb := &Block{
					kind:      Token{str: word, typ: _keyword_t},
					target:    Token{},
					operator:  Token{},
					typevalue: Token{},
					state:     _l2_kind_done,
					child:     []*Block{},
					parent:    block,
					variable:  vsys{vars: make(map[string]*_var)},
					bbody:     body,
				}
				block.child = append(block.child, newb)
				block = newb
				break
			}
			// error
			return errors.New("contain invalid character: " + newline)
		case _l1_load_started:
			///
			// load go:sleep
			//
			switch block.state {
			case _l2_kind_done:
				// skip
				if is.Space(current) {
					break
				}
				// transfer
				if is.Word(current) {
					start = last
					block.state = _l2_target_started
					break
				}
				// error
				return errors.New("contain invalid character: " + newline)
			case _l2_target_started:
				// transfer
				if is.EOL(current) {
					block.target = Token{
						str: strings.TrimSpace(newline[start:last]),
						typ: _load_t,
					}
					block.state = _l2_target_done
					ast.transfer(_l1_global)
					block = block.parent
					break
				}
			case _l2_target_done:

			}
		case _l1_run_started:
			/**
			 1. run sleep
			 2. run {
			 		f1
					f2
			 	}
			3. run sleep {
				time: 1s
			}
			*/
			switch block.state {
			case _l2_kind_done:
				// skip
				if is.Space(current) {
					break
				}
				// transfer 1
				if is.Word(current) {
					start = last
					block.state = _l2_target_started
					break
				}
				if is.LeftBracket(current) {
					/*
						run {
							f1
							f2
						}
					*/
					block.bbody = &FList{etype: _functionname_t}
					ast.transfer(_l1_run_body_started)
					break
				}
				// error
				return errors.New("contain invalid character: " + newline)
			case _l2_target_started:
				// keep
				if is.Word(current) {
					break
				}
				// 1. transfer - run sleep{
				if is.LeftBracket(current) {
					block.target = Token{
						str: strings.TrimSpace(newline[start:last]),
						typ: _functionname_t,
					}
					block.bbody = &FMap{}
					block.state = _l2_unknow
					ast.transfer(_l1_run_body_started)
					break
				}
				// 2. transfer - run sleep {  or run sleep
				if is.Space(current) {
					block.target = Token{
						str: strings.TrimSpace(newline[start:last]),
						typ: _functionname_t,
					}
					block.state = _l2_target_done
					break
				}
				// 3. transfer run sleep
				if is.EOL(current) {
					block.target = Token{
						str: strings.TrimSpace(newline[start:last]),
						typ: _functionname_t,
					}
					block.state = _l2_unknow
					ast.transfer(_l1_global)
					block = block.parent
					break
				}
				return errors.New("contain invalid character: " + newline)
			case _l2_target_done:
				// transfer
				if is.EOL(current) {
					ast.transfer(_l1_global)
					block = block.parent
					break
				}
				if is.LeftBracket(current) {
					block.bbody = &FMap{}
					ast.transfer(_l1_run_body_started)
					break
				}
				// skip
				if is.Space(current) {
					break
				}
				// error
				return errors.New("contain invalid character: " + newline)
			}

		case _l1_run_body_started:
			// transfer
			if is.EOL(current) {
				ast.transfer(_l1_run_body_inside)
				break
			}
			// skip
			if is.Space(current) {
				break
			}
			return errors.New("invalid run block: " + newline + fmt.Sprintf(" (%c)", current))
		case _l1_run_body_inside:
			// 1. k: v
			// 2. f
			// 3. }
			if is.EOL(current) {
				if newline == "}" {
					ast.transfer(_l1_global)
					block = block.parent
				} else if newline != "" {
					if err := block.bbody.Append(newline); err != nil {
						return err
					}
				}
			}

		case _l1_fn_started:
			/*
				fn f1 = f {

				}
			*/
			switch block.state {
			case _l2_kind_done:
				// skip
				if is.Space(current) {
					break
				}
				// transfer
				if is.Word(current) {
					start = last
					block.state = _l2_target_started
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			case _l2_target_started: // from '{word}'
				// keep
				if is.Word(current) {
					break
				}
				if is.Space(current) {
					break
				}
				// transfer
				if is.Eq(current) {
					s := newline[start:last]
					block.target = Token{
						str: strings.TrimSpace(s),
						typ: _word_t,
					}
					block.operator = Token{
						str: "=",
						typ: _operator_t,
					}
					block.state = _l2_operator_started
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			case _l2_operator_started: // from '='
				// skip
				if is.Space(current) {
					break
				}
				// transfer
				if is.Word(current) {
					start = last
					block.state = _l2_typevalue_started
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			case _l2_typevalue_started: // from '{word}'
				// keep
				if is.Word(current) {
					break
				}
				if is.Space(current) {
					break
				}
				// transfer
				if is.LeftBracket(current) {
					s := newline[start:last]
					block.typevalue = Token{
						str: strings.TrimSpace(s),
						typ: _functionname_t,
					}
					block.state = _l2_unknow
					ast.transfer(_l1_fn_body_started)
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			}
		case _l1_fn_body_started: // from '{'
			// skip
			if is.Space(current) {
				break
			}
			// transfer
			if is.EOL(current) {
				block.state = _l2_unknow
				ast.transfer(_l1_fn_body_inside)
				break
			}
			// error
			return errInvalidChar(byte(current), newline)
		case _l1_fn_body_inside: // from '\n'
			if block.state == _l2_word_started {
				if unicode.IsSpace(current) || current == '=' {
					block.state = _l2_unknow
					s := newline[start:last]
					switch s {
					case "args":
						newb := &Block{
							kind:      Token{str: s, typ: _word_t},
							target:    Token{},
							operator:  Token{},
							typevalue: Token{},
							state:     _l2_kind_done,
							child:     []*Block{},
							parent:    block,
							variable:  vsys{},
							bbody:     &FMap{},
						}
						block.child = append(block.child, newb)
						block = newb
						ast.transfer(_l1_args_started)
					default:
						return errors.New("invalid statement in fn block: " + newline)
					}
				}
			} else {
				// the right bracket of fn block body is appeared, so fn block should be closed
				if current == '\n' && newline == "}" {
					block.state = _l2_unknow
					ast.transfer(_l1_global)
					block = block.parent
					break
				}
				if unicode.IsSpace(current) || current == '}' {
					break
				}
				start = last
				block.state = _l2_word_started
			}
		case _l1_args_started:
			switch block.state {
			case _l2_kind_done:
				if unicode.IsSpace(current) {
					break
				}
				if current == '=' {
					block.state = _l2_operator_started
				} else {
					return errors.New("invliad args block: " + newline)
				}
			case _l2_operator_started:
				if current == '{' || unicode.IsSpace(current) {
					block.operator = Token{
						str: "=",
						typ: _operator_t,
					}
					block.state = _l2_operator_done
					if current == '{' {
						ast.transfer(_l1_args_body_started)
					}
				} else {
					return errors.New("invalid args block: " + newline)
				}
			case _l2_operator_done:
				if unicode.IsSpace(current) {
					break
				}
				if current == '{' {
					ast.transfer(_l1_args_body_started)
				} else {
					return errors.New("invalid args block: " + newline)
				}
			}
		case _l1_args_body_started:
			if current == '\n' {
				ast.transfer(_l1_args_body_inside)
				break
			}
			if !unicode.IsSpace(current) {
				return errors.New("invalid args block: " + newline)
			}
		case _l1_args_body_inside:
			if current == '\n' {
				if newline == "}" {
					block = block.parent
					block.state = _l2_unknow
					ast.transfer(_l1_fn_body_inside)
				} else {
					if err := block.bbody.Append(newline); err != nil {
						return err
					}
				}
			}
		case _l1_var_started:
			// var a = 1
			// var a = $(b)
			switch block.state {
			case _l2_kind_done:
				// skip
				if is.Space(current) {
					break
				}
				// transfer
				if is.Word(current) {
					start = last
					block.state = _l2_target_started
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			case _l2_target_started: // from '{word}'
				// keep
				if is.Word(current) {
					break
				}
				if is.Space(current) {
					break
				}
				// transfer
				// 1. var a
				if is.EOL(current) {
					s := newline[start:last]
					stm := block.bbody.(*plainbody).Laststm()
					stm.Append(&Token{
						str: strings.TrimSpace(s),
						typ: _varname_t,
						_b:  block,
					})
					block.state = _l2_unknow
					ast.transfer(_l1_global)
					break
				}

				// 2. var a = 1
				if is.Eq(current) {
					s := newline[start:last]
					stm := block.bbody.(*plainbody).Laststm()
					stm.Append(&Token{
						str: strings.TrimSpace(s),
						typ: _varname_t,
						_b:  block,
					})
					block.state = _l2_operator_started
					break
				}
				// error
				return errInvalidChar(byte(current), newline)
			case _l2_operator_started: // from '='
				// skip
				if is.Space(current) {
					break
				}
				// not space, transfer
				start = last
				block.state = _l2_typevalue_started
			case _l2_typevalue_started: // from '{word}'
				// keep

				// transfer
				if is.EOL(current) {
					s := newline[start:last]
					stm := block.bbody.(*plainbody).Laststm()
					stm.Append(&Token{
						str: strings.TrimSpace(s),
						typ: _text_t,
						_b:  block,
					})
					block.state = _l2_unknow
					ast.transfer(_l1_global)
					break
				}
				// error
			}

		default:
		}
		return nil
	}

	line = strings.TrimSpace(line)
	for i, c := range line {
		if err := finiteAutomata(i, c, line); err != nil {
			return err
		}
	}
	// todo, comment
	if err := finiteAutomata(len(line), '\n', line); err != nil {
		return err
	}
	ast.parsing = block
	return nil
}

// AST store all blocks in the flowl
//
type AST struct {
	global Block

	// for parsing
	_FA
}

func newAST() *AST {
	ast := &AST{
		global: Block{
			kind:      Token{},
			target:    Token{},
			operator:  Token{},
			typevalue: Token{},
			state:     _l2_unknow,
			child:     make([]*Block, 0),
			parent:    nil,
			variable:  vsys{vars: make(map[string]*_var)},
			bbody:     &plainbody{},
		},
		_FA: _FA{
			state: _l1_global,
		},
	}
	ast._FA.parsing = &ast.global
	return ast
}

func deepwalk(b *Block, do func(*Block) error) error {
	if err := do(b); err != nil {
		return err
	}
	for _, c := range b.child {
		if err := deepwalk(c, do); err != nil {
			return err
		}
	}
	return nil
}

func (a *AST) Foreach(do func(*Block) error) error {
	return deepwalk(&a.global, do)
}

type stateL1 int
type stateL2 int

type _FA struct {
	parsing  *Block
	state    stateL1
	prestate stateL1
}

func (f *_FA) transfer(s stateL1) {
	f.prestate = f.state
	f.state = s
}

func (f *_FA) Back() {
}

func (f *_FA) phase() stateL1 {
	return f.state
}
