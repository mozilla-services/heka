%{
package message

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"sync"
	"unicode/utf8"
)

const (
	STARTS_WITH = 1
	ENDS_WITH   = 2
)

var variables = map[string]int{
	"Uuid":       VAR_UUID,
	"Type":       VAR_TYPE,
	"Logger":     VAR_LOGGER,
	"Payload":    VAR_PAYLOAD,
	"EnvVersion": VAR_ENVVERSION,
	"Hostname":   VAR_HOSTNAME,
	"Timestamp":  VAR_TIMESTAMP,
	"Severity":   VAR_SEVERITY,
	"Pid":        VAR_PID,
	"Fields":     VAR_FIELDS,
	"TRUE":       TRUE,
	"FALSE":      FALSE,
	"NIL":        NIL_VALUE}

var parseLock sync.Mutex

type Statement struct {
	field, op, value yySymType
}

type tree struct {
	left  *tree
	stmt  *Statement
	right *tree
}

type stack struct {
	top  *item
	size int
}

type item struct {
	node *tree
	next *item
}

func (s *stack) push(node *tree) {
	s.top = &item{node, s.top}
	s.size++
}

func (s *stack) pop() (node *tree) {
	if s.size > 0 {
		node, s.top = s.top.node, s.top.next
		s.size--
		return
	}
	return nil
}

var nodes []*tree

%}

%union {
   tokenId     int
   token       string
   double      float64
   fieldIndex  int
   arrayIndex  int
   regexp      *regexp.Regexp
}

%token OP_EQ OP_NE OP_GT OP_GTE OP_LT OP_LTE OP_RE OP_NRE
%token OP_OR OP_AND
%token VAR_UUID VAR_TYPE VAR_LOGGER VAR_PAYLOAD VAR_ENVVERSION VAR_HOSTNAME
%token VAR_TIMESTAMP VAR_SEVERITY VAR_PID
%token VAR_FIELDS
%token STRING_VALUE NUMERIC_VALUE REGEXP_VALUE NIL_VALUE
%token TRUE FALSE

%start spec
%left OP_OR
%left OP_AND

%%

spec : expr
   | spec expr
;
relational : OP_EQ
   | OP_NE
   | OP_GT
   | OP_GTE
   | OP_LT
   | OP_LTE
;
eqneq : OP_EQ
   | OP_NE
;
regexp : OP_RE
   | OP_NRE
;
string_vars : VAR_UUID
   | VAR_TYPE
   | VAR_LOGGER
   | VAR_PAYLOAD
   | VAR_ENVVERSION
   | VAR_HOSTNAME
;
numeric_vars : VAR_TIMESTAMP
   | VAR_SEVERITY
   | VAR_PID
;
string_test : string_vars relational STRING_VALUE
       {
       //fmt.Println("string_test", $1, $2, $3)
       nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
       }
   |   string_vars regexp REGEXP_VALUE
       {
       //fmt.Println("string_test regexp", $1, $2, $3)
       nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
       }
;
numeric_test : numeric_vars relational NUMERIC_VALUE
   {
   //fmt.Println("numeric_test", $1, $2, $3)
   nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
   }
;
field_test : VAR_FIELDS relational NUMERIC_VALUE
      {
      //fmt.Println("field_test numeric", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
      }
   | VAR_FIELDS relational STRING_VALUE
      {
      //fmt.Println("field_test string", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
      }
   | VAR_FIELDS OP_EQ boolean
      {
      //fmt.Println("field_test boolean", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
      }
   | VAR_FIELDS regexp REGEXP_VALUE
      {
      //fmt.Println("field_test regexp", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
      }
   | VAR_FIELDS eqneq NIL_VALUE
      {
      //fmt.Println("field_test existence", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{$1, $2, $3}})
      }
;
boolean : TRUE | FALSE
expr : '(' expr ')'
      {
      $$ = $2
      }
   | expr OP_AND expr
      {
      //fmt.Println("and", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{op:$2}})
      }
   | expr OP_OR expr
      {
      //fmt.Println("or", $1, $2, $3)
      nodes = append(nodes, &tree{stmt:&Statement{op:$2}})
      }
   | string_test
   | numeric_test
   | field_test
   | boolean
      {
         //fmt.Println("boolean", $1)
         nodes = append(nodes, &tree{stmt:&Statement{op:$1}})
      }
;

%%

type MatcherSpecificationParser struct {
	spec     string
	sym      string
	peekrune rune
	lexPos   int
    reToken *regexp.Regexp
}

func parseMatcherSpecification(ms *MatcherSpecification) error {
	parseLock.Lock()
	defer parseLock.Unlock()
	nodes = nodes[:0] // reset the global
	var msp MatcherSpecificationParser
	msp.spec = ms.spec
	msp.peekrune = ' '
	msp.reToken, _ = regexp.Compile("%[A-Z]+%")
	if yyParse(&msp) == 0 {
		s := new(stack)
		for _, node := range nodes {
			if node.stmt.op.tokenId != OP_OR &&
				node.stmt.op.tokenId != OP_AND {
				s.push(node)
			} else {
				node.right = s.pop()
				node.left = s.pop()
				s.push(node)
			}
		}
		ms.vm = s.pop()
		return nil
	}
	return fmt.Errorf("syntax error: last token: %s pos: %d", msp.sym, msp.lexPos)
}

func (m *MatcherSpecificationParser) Error(s string) {
	fmt.Errorf("syntax error: %s last token: %s pos: %d", m.sym, m.lexPos)
}

func (m *MatcherSpecificationParser) Lex(yylval *yySymType) int {
	var err error
	var c, tmp rune
	var i int

	yylval.tokenId = 0
	yylval.token = ""
	yylval.double = 0
	yylval.fieldIndex = 0
	yylval.arrayIndex = 0
	yylval.regexp = nil

	c = m.peekrune
	m.peekrune = ' '

loop:
	if c >= 'A' && c <= 'Z' {
		goto variable
	}
	if (c >= '0' && c <= '9') || c == '.' {
		goto number
	}
	switch c {
	case ' ', '\t':
		c = m.getrune()
		goto loop
	case '=':
		c = m.getrune()
		if c == '=' {
			yylval.token = "=="
			yylval.tokenId = OP_EQ
		} else if c == '~' {
			yylval.token = "=~"
			yylval.tokenId = OP_RE
		} else {
			break
		}
		return yylval.tokenId
	case '!':
		c = m.getrune()
		if c == '=' {
			yylval.token = "!="
			yylval.tokenId = OP_NE
		} else if c == '~' {
			yylval.token = "!~"
			yylval.tokenId = OP_NRE
		} else {
			break
		}
		return yylval.tokenId
	case '>':
		c = m.getrune()
		if c != '=' {
			m.peekrune = c
			yylval.token = ">"
			yylval.tokenId = OP_GT
			return yylval.tokenId
		}
		yylval.token = ">="
		yylval.tokenId = OP_GTE
		return yylval.tokenId
	case '<':
		c = m.getrune()
		if c != '=' {
			m.peekrune = c
			yylval.token = "<"
			yylval.tokenId = OP_LT
			return yylval.tokenId
		}
		yylval.token = "<="
		yylval.tokenId = OP_LTE
		return yylval.tokenId
	case '|':
		c = m.getrune()
		if c != '|' {
			break
		}
		yylval.token = "||"
		yylval.tokenId = OP_OR
		return yylval.tokenId
	case '&':
		c = m.getrune()
		if c != '&' {
			break
		}
		yylval.token = "&&"
		yylval.tokenId = OP_AND
		return yylval.tokenId
	case '"', '\'':
		goto quotestring
	case '/':
		goto regexpstring
	}
	return int(c)

variable:
	m.sym = ""
	for i = 0; ; i++ {
		m.sym += string(c)
		c = m.getrune()
		if !rvariable(c) {
			break
		}
	}
	yylval.tokenId = variables[m.sym]
	if yylval.tokenId == VAR_FIELDS {
		if c != '[' {
			return 0
		}
		var bracketCount int
		var idx [3]string
		for {
			c = m.getrune()
			if c == 0 {
				return 0
			}
			if c == ']' { // a closing bracket in the variable name will fail validation
				if len(idx[bracketCount]) == 0 {
					return 0
				}
				bracketCount++
				m.peekrune = m.getrune()
				if m.peekrune == '[' && bracketCount < cap(idx) {
					m.peekrune = ' '
				} else {
					break
				}
			} else {
				switch bracketCount {
				case 0:
					idx[bracketCount] += string(c)
				case 1, 2:
					if ddigit(c) {
						idx[bracketCount] += string(c)
					} else {
						return 0
					}
				}
			}
		}
		if len(idx[1]) == 0 {
			idx[1] = "0"
		}
		if len(idx[2]) == 0 {
			idx[2] = "0"
		}
		var err error
		yylval.token = idx[0]
		yylval.fieldIndex, err = strconv.Atoi(idx[1])
		if err != nil {
			return 0
		}
		yylval.arrayIndex, err = strconv.Atoi(idx[2])
		if err != nil {
			return 0
		}
	} else {
		yylval.token = m.sym
		m.peekrune = c
	}
	return yylval.tokenId

number:
	m.sym = ""
	for i = 0; ; i++ {
		m.sym += string(c)
		c = m.getrune()
		if !rdigit(c) {
			break
		}
	}
	m.peekrune = c
	yylval.double, err = strconv.ParseFloat(m.sym, 64)
	if err != nil {
		log.Printf("error converting %v\n", m.sym)
		yylval.double = 0
	}
	yylval.token = m.sym
	yylval.tokenId = NUMERIC_VALUE
	return yylval.tokenId

quotestring:
	tmp = c
	m.sym = ""
	for {
		c = m.getrune()
		if c == 0 {
			return 0
		}
		if c == '\\' {
			m.peekrune = m.getrune()
			if m.peekrune == tmp {
				m.sym += string(tmp)
			} else {
				m.sym += string(c)
				m.sym += string(m.peekrune)
			}
			m.peekrune = ' '
			continue
		}
		if c == tmp {
			break
		}
		m.sym += string(c)
	}
	yylval.token = m.sym
	yylval.tokenId = STRING_VALUE
	return yylval.tokenId

regexpstring:
	m.sym = ""
	for {
		c = m.getrune()
		if c == 0 {
			return 0
		}
		if c == '\\' {
			m.peekrune = m.getrune()
			if m.peekrune == '/' {
				m.sym += "/"
			} else {
				m.sym += string(c)
				m.sym += string(m.peekrune)
			}
			m.peekrune = ' '
			continue
		}
		if c == '/' {
			break
		}
		m.sym += string(c)
	}
	rlen := len(m.sym)
	if rlen > 0 && m.sym[0] == '^' {
		if re, err := regexp.Compile(m.sym[1:]); err == nil {
			if s, b := re.LiteralPrefix(); b {
				yylval.token = s
				yylval.fieldIndex = STARTS_WITH
				yylval.tokenId = REGEXP_VALUE
				return yylval.tokenId
			}
		}
	}
	if rlen > 0 && m.sym[rlen-1] == '$' {
		if re, err := regexp.Compile(m.sym[:rlen-1]); err == nil {
			if s, b := re.LiteralPrefix(); b {
				yylval.token = s
				yylval.fieldIndex = ENDS_WITH
				yylval.tokenId = REGEXP_VALUE
				return yylval.tokenId
			}
		}
	}
	yylval.regexp, err = regexp.Compile(m.sym)
	if err != nil {
		log.Printf("invalid regexp %v\n", m.sym)
		return 0
	}
	yylval.token = m.sym
	yylval.tokenId = REGEXP_VALUE
	return yylval.tokenId
}

func rvariable(c rune) bool {
	if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
		return true
	}
	return false
}

func rdigit(c rune) bool {
	switch c {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'.', 'e', '+', '-':
		return true
	}
	return false
}

func ddigit(c rune) bool {
	switch c {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}
	return false
}

func (m *MatcherSpecificationParser) getrune() rune {
	var c rune
	var n int

	if m.lexPos >= len(m.spec) {
		return 0
	}
	c, n = utf8.DecodeRuneInString(m.spec[m.lexPos:len(m.spec)])
	m.lexPos += n
	if c == '\n' {
		c = 0
	}
	return c
}
