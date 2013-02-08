//line filter_specification_parser.y:2
package pipeline

import __yyfmt__ "fmt"

//line filter_specification_parser.y:2
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

//line filter_specification_parser.y:65
type yySymType struct {
	yys     int
	tokenId int
	token   string
	double  float64
}

const OP_EQ = 57346
const OP_NE = 57347
const OP_GT = 57348
const OP_GTE = 57349
const OP_LT = 57350
const OP_LTE = 57351
const OP_OR = 57352
const OP_AND = 57353
const VAR_UUID = 57354
const VAR_TYPE = 57355
const VAR_LOGGER = 57356
const VAR_PAYLOAD = 57357
const VAR_ENVVERSION = 57358
const VAR_HOSTNAME = 57359
const VAR_TIMESTAMP = 57360
const VAR_SEVERITY = 57361
const VAR_PID = 57362
const STRING_VALUE = 57363
const NUMERIC_VALUE = 57364
const TRUE = 57365
const FALSE = 57366

var yyToknames = []string{
	"OP_EQ",
	"OP_NE",
	"OP_GT",
	"OP_GTE",
	"OP_LT",
	"OP_LTE",
	"OP_OR",
	"OP_AND",
	"VAR_UUID",
	"VAR_TYPE",
	"VAR_LOGGER",
	"VAR_PAYLOAD",
	"VAR_ENVVERSION",
	"VAR_HOSTNAME",
	"VAR_TIMESTAMP",
	"VAR_SEVERITY",
	"VAR_PID",
	"STRING_VALUE",
	"NUMERIC_VALUE",
	"TRUE",
	"FALSE",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line filter_specification_parser.y:141
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
	f.peekrune = c
	yylval.token = f.sym
	yylval.tokenId = variables[f.sym]
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

//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
}

const yyNprod = 28
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 39

var yyAct = []int{

	11, 12, 13, 14, 15, 16, 17, 18, 19, 22,
	21, 9, 10, 3, 2, 36, 20, 35, 23, 25,
	26, 27, 28, 29, 30, 34, 22, 21, 21, 24,
	6, 5, 4, 8, 7, 1, 32, 33, 31,
}
var yyPact = []int{

	-12, -12, 16, -12, -1000, -1000, -1000, 15, 15, -1000,
	-1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000,
	16, -12, -12, -1, -4, -1000, -1000, -1000, -1000, -1000,
	-1000, -7, -1000, 17, -1000, -1000, -1000,
}
var yyPgo = []int{

	0, 35, 14, 29, 34, 33, 32, 31, 30,
}
var yyR1 = []int{

	0, 1, 1, 3, 3, 3, 3, 3, 3, 4,
	4, 4, 4, 4, 4, 5, 5, 5, 6, 7,
	8, 8, 2, 2, 2, 2, 2, 2,
}
var yyR2 = []int{

	0, 1, 2, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 3, 3,
	1, 1, 3, 3, 3, 1, 1, 1,
}
var yyChk = []int{

	-1000, -1, -2, 25, -6, -7, -8, -4, -5, 23,
	24, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	-2, 11, 10, -2, -3, 4, 5, 6, 7, 8,
	9, -3, -2, -2, 26, 21, 22,
}
var yyDef = []int{

	0, -2, 1, 0, 25, 26, 27, 0, 0, 20,
	21, 9, 10, 11, 12, 13, 14, 15, 16, 17,
	2, 0, 0, 0, 0, 3, 4, 5, 6, 7,
	8, 0, 23, 24, 22, 18, 19,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	25, 26,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24,
}
var yyTok3 = []int{
	0,
}

//line yaccpar:1

/*	parser for yacc output	*/

var yyDebug = 0

type yyLexer interface {
	Lex(lval *yySymType) int
	Error(s string)
}

const yyFlag = -1000

func yyTokname(c int) string {
	if c > 0 && c <= len(yyToknames) {
		if yyToknames[c-1] != "" {
			return yyToknames[c-1]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func yylex1(lex yyLexer, lval *yySymType) int {
	c := 0
	char := lex.Lex(lval)
	if char <= 0 {
		c = yyTok1[0]
		goto out
	}
	if char < len(yyTok1) {
		c = yyTok1[char]
		goto out
	}
	if char >= yyPrivate {
		if char < yyPrivate+len(yyTok2) {
			c = yyTok2[char-yyPrivate]
			goto out
		}
	}
	for i := 0; i < len(yyTok3); i += 2 {
		c = yyTok3[i+0]
		if c == char {
			c = yyTok3[i+1]
			goto out
		}
	}

out:
	if c == 0 {
		c = yyTok2[1] /* unknown char */
	}
	if yyDebug >= 3 {
		__yyfmt__.Printf("lex %U %s\n", uint(char), yyTokname(c))
	}
	return c
}

func yyParse(yylex yyLexer) int {
	var yyn int
	var yylval yySymType
	var yyVAL yySymType
	yyS := make([]yySymType, yyMaxDepth)

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	yystate := 0
	yychar := -1
	yyp := -1
	goto yystack

ret0:
	return 0

ret1:
	return 1

yystack:
	/* put a state and value onto the stack */
	if yyDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", yyTokname(yychar), yyStatname(yystate))
	}

	yyp++
	if yyp >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyS[yyp] = yyVAL
	yyS[yyp].yys = yystate

yynewstate:
	yyn = yyPact[yystate]
	if yyn <= yyFlag {
		goto yydefault /* simple state */
	}
	if yychar < 0 {
		yychar = yylex1(yylex, &yylval)
	}
	yyn += yychar
	if yyn < 0 || yyn >= yyLast {
		goto yydefault
	}
	yyn = yyAct[yyn]
	if yyChk[yyn] == yychar { /* valid shift */
		yychar = -1
		yyVAL = yylval
		yystate = yyn
		if Errflag > 0 {
			Errflag--
		}
		goto yystack
	}

yydefault:
	/* default state action */
	yyn = yyDef[yystate]
	if yyn == -2 {
		if yychar < 0 {
			yychar = yylex1(yylex, &yylval)
		}

		/* look through exception table */
		xi := 0
		for {
			if yyExca[xi+0] == -1 && yyExca[xi+1] == yystate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			yyn = yyExca[xi+0]
			if yyn < 0 || yyn == yychar {
				break
			}
		}
		yyn = yyExca[xi+1]
		if yyn < 0 {
			goto ret0
		}
	}
	if yyn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			yylex.Error("syntax error")
			Nerrs++
			if yyDebug >= 1 {
				__yyfmt__.Printf("%s", yyStatname(yystate))
				__yyfmt__.Printf("saw %s\n", yyTokname(yychar))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for yyp >= 0 {
				yyn = yyPact[yyS[yyp].yys] + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = yyAct[yyn] /* simulate a shift of "error" */
					if yyChk[yystate] == yyErrCode {
						goto yystack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if yyDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", yyTokname(yychar))
			}
			if yychar == yyEofCode {
				goto ret1
			}
			yychar = -1
			goto yynewstate /* try again in the same state */
		}
	}

	/* reduction by production yyn */
	if yyDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
	}

	yynt := yyn
	yypt := yyp
	_ = yypt // guard against "declared and not used"

	yyp -= yyR2[yyn]
	yyVAL = yyS[yyp+1]

	/* consult goto table to find next state */
	yyn = yyR1[yyn]
	yyg := yyPgo[yyn]
	yyj := yyg + yyS[yyp].yys + 1

	if yyj >= yyLast {
		yystate = yyAct[yyg]
	} else {
		yystate = yyAct[yyj]
		if yyChk[yystate] != -yyn {
			yystate = yyAct[yyg]
		}
	}
	// dummy call; replaced with literal code
	switch yynt {

	case 18:
		//line filter_specification_parser.y:106
		{
			//fmt.Println("string_test", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 19:
		//line filter_specification_parser.y:112
		{
			//fmt.Println("numeric_test", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 22:
		//line filter_specification_parser.y:119
		{
			yyVAL = yyS[yypt-1]
		}
	case 23:
		//line filter_specification_parser.y:123
		{
			//fmt.Println("and", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-1]}})
		}
	case 24:
		//line filter_specification_parser.y:128
		{
			//fmt.Println("or", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-1]}})
		}
	case 27:
		//line filter_specification_parser.y:135
		{
			//fmt.Println("boolean", $1)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-0]}})
		}
	}
	goto yystack /* stack new state and value */
}
