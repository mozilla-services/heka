%{
package pipeline

import (
	"fmt"
	"log"
	"strconv"
   "sync"
	"unicode/utf8"
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
	"FALSE":      FALSE}

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
}

%token OP_EQ OP_NE OP_GT OP_GTE OP_LT OP_LTE
%token OP_OR OP_AND
%token VAR_UUID VAR_TYPE VAR_LOGGER VAR_PAYLOAD VAR_ENVVERSION VAR_HOSTNAME
%token VAR_TIMESTAMP VAR_SEVERITY VAR_PID
%token VAR_FIELDS
%token STRING_VALUE NUMERIC_VALUE
%token TRUE FALSE

%start filter
%left OP_OR
%left OP_AND

%%

filter : expr
   | filter expr
;
relational : OP_EQ
   | OP_NE
   | OP_GT
   | OP_GTE
   | OP_LT
   | OP_LTE
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

type FilterSpecificationParser struct {
   filter   string
	sym      string
	peekrune rune
   lexPos   int
}

func parseFilterSpecification(fs *FilterSpecification) error {
	parseLock.Lock()
   defer parseLock.Unlock()
   nodes = nodes[:0] // reset the global
   var fsp FilterSpecificationParser
   fsp.filter = fs.filter
   fsp.peekrune = ' '
	if yyParse(&fsp) == 0 {
	   s := new(stack)
   	for _, node := range nodes {
   		if node.stmt.op.tokenId != OP_OR && node.stmt.op.tokenId != OP_AND {
   			s.push(node)
   		} else {
            node.right = s.pop()
            node.left = s.pop()
            s.push(node)
   		}
   	}
      fs.vm = s.pop()
		return nil
	}
	return fmt.Errorf("syntax error: last token: %s pos: %d", fsp.sym, fsp.lexPos)
}

func (f *FilterSpecificationParser) Error(s string) {
	fmt.Errorf("syntax error: %s last token: %s pos: %d", f.sym, f.lexPos)
}

func (f *FilterSpecificationParser) Lex(yylval *yySymType) int {
	var err error
	var c rune
	var i int

	c = f.peekrune
	f.peekrune = ' '

loop:
	if c >= 'A' && c <= 'Z' {
		goto variable
	}
	if (c >= '0' && c <= '9') || c == '.' {
		goto number
	}
	switch c {
	case ' ', '\t':
		c = f.getrune()
		goto loop
	case '=':
		c = f.getrune()
		if c != '=' {
			break
		}
		yylval.token = "=="
		yylval.tokenId = OP_EQ
		return yylval.tokenId
	case '!':
		c = f.getrune()
		if c != '=' {
			break
		}
		yylval.token = "!="
		yylval.tokenId = OP_NE
		return yylval.tokenId
	case '>':
		c = f.getrune()
		if c != '=' {
			f.peekrune = c
			yylval.token = ">"
			yylval.tokenId = OP_GT
			return yylval.tokenId
		}
		f.peekrune = f.getrune()
		yylval.token = ">="
		yylval.tokenId = OP_GTE
		return yylval.tokenId
	case '<':
		c = f.getrune()
		if c != '=' {
			f.peekrune = c
			yylval.token = "<"
			yylval.tokenId = OP_LT
			return yylval.tokenId
		}
		yylval.token = "<="
		yylval.tokenId = OP_LTE
		return yylval.tokenId
	case '|':
		c = f.getrune()
		if c != '|' {
			break
		}
		yylval.token = "||"
		yylval.tokenId = OP_OR
		return yylval.tokenId
	case '&':
		c = f.getrune()
		if c != '&' {
			break
		}
		yylval.token = "&&"
		yylval.tokenId = OP_AND
		return yylval.tokenId
	case '"':
		goto quotestring
	}
	return int(c)

variable:
	f.sym = ""
	for i = 0; ; i++ {
		f.sym += string(c)
		c = f.getrune()
		if !rvariable(c) {
			break
		}
	}
	yylval.tokenId = variables[f.sym]
   if yylval.tokenId == VAR_FIELDS {
      if c != '[' {
         return 0
      }
      var bracketCount int
      var idx [3]string
      for {
      	c = f.getrune()
      	if c == 0 {
      		return 0
      	}
      	if c == ']' { // a closing bracket in the variable name will fail validation
            if len(idx[bracketCount]) == 0 {
               return 0
            }
            bracketCount++
			   f.peekrune = f.getrune()
   			if f.peekrune == '[' && bracketCount < cap(idx) {
   			   f.peekrune = ' '
   			} else {
               break
            }
      	} else {
            switch bracketCount {
               case 0:
                  idx[bracketCount] += string(c)
               case 1,2:
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
	   yylval.token = f.sym
	   f.peekrune = c
   }
	return yylval.tokenId

number:
	f.sym = ""
	for i = 0; ; i++ {
		f.sym += string(c)
		c = f.getrune()
		if !rdigit(c) {
			break
		}
	}
	f.peekrune = c
	yylval.double, err = strconv.ParseFloat(f.sym, 64)
	if err != nil {
		log.Printf("error converting %v\n", f.sym)
		yylval.double = 0
	}
	yylval.token = f.sym
	yylval.tokenId = NUMERIC_VALUE
	return yylval.tokenId

quotestring:
	f.sym = ""
	for {
		c = f.getrune()
		if c == 0 {
			return 0
		}
		if c == '\\' {
			f.peekrune = f.getrune()
			if f.peekrune == '"' {
				f.sym += "\""
			} else {
				f.sym += string(c)
				f.sym += string(f.peekrune)
			}
			f.peekrune = ' '
			continue
		}
		if c == '"' {
			break
		}
		f.sym += string(c)
	}
	yylval.token = f.sym
	yylval.tokenId = STRING_VALUE
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

func (f *FilterSpecificationParser) getrune() rune {
	var c rune
	var n int

	if f.lexPos >= len(f.filter) {
		return 0
	}
	c, n = utf8.DecodeRuneInString(f.filter[f.lexPos:len(f.filter)])
	f.lexPos += n
	if c == '\n' {
		c = 0
	}
	return c
}
