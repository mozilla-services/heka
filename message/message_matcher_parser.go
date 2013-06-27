//line message_matcher_parser.y:2
package message

import __yyfmt__ "fmt"

//line message_matcher_parser.y:2
import (
	"fmt"
	"log"
	"regexp"
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

//line message_matcher_parser.y:67
type yySymType struct {
	yys        int
	tokenId    int
	token      string
	double     float64
	fieldIndex int
	arrayIndex int
	regexp     *regexp.Regexp
}

const OP_EQ = 57346
const OP_NE = 57347
const OP_GT = 57348
const OP_GTE = 57349
const OP_LT = 57350
const OP_LTE = 57351
const OP_RE = 57352
const OP_NRE = 57353
const OP_OR = 57354
const OP_AND = 57355
const VAR_UUID = 57356
const VAR_TYPE = 57357
const VAR_LOGGER = 57358
const VAR_PAYLOAD = 57359
const VAR_ENVVERSION = 57360
const VAR_HOSTNAME = 57361
const VAR_TIMESTAMP = 57362
const VAR_SEVERITY = 57363
const VAR_PID = 57364
const VAR_FIELDS = 57365
const STRING_VALUE = 57366
const NUMERIC_VALUE = 57367
const REGEXP_VALUE = 57368
const TRUE = 57369
const FALSE = 57370

var yyToknames = []string{
	"OP_EQ",
	"OP_NE",
	"OP_GT",
	"OP_GTE",
	"OP_LT",
	"OP_LTE",
	"OP_RE",
	"OP_NRE",
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
	"VAR_FIELDS",
	"STRING_VALUE",
	"NUMERIC_VALUE",
	"REGEXP_VALUE",
	"TRUE",
	"FALSE",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line message_matcher_parser.y:177
type MatcherSpecificationParser struct {
	spec     string
	sym      string
	peekrune rune
	lexPos   int
	reToken  *regexp.Regexp
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
	m.sym = m.reToken.ReplaceAllStringFunc(m.sym,
		func(match string) string {
			replace, ok := HelperRegexSubs[match[1:len(match)-1]]
			if !ok {
				return match
			}
			return replace
		})
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

//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
}

const yyNprod = 36
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 77

var yyAct = []int{

	13, 14, 15, 16, 17, 18, 19, 20, 21, 10,
	7, 24, 23, 11, 12, 3, 11, 12, 2, 49,
	22, 44, 25, 45, 47, 46, 43, 24, 23, 42,
	38, 29, 30, 31, 32, 33, 34, 35, 23, 6,
	5, 4, 40, 41, 9, 8, 1, 0, 0, 48,
	28, 29, 30, 31, 32, 33, 34, 35, 28, 29,
	30, 31, 32, 33, 26, 27, 0, 0, 0, 0,
	0, 0, 0, 0, 36, 37, 39,
}
var yyPact = []int{

	-14, -14, 15, -14, -1000, -1000, -1000, -1000, 46, 54,
	26, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000,
	-1000, -1000, 15, -14, -14, -1, 2, -5, -1000, -1000,
	-1000, -1000, -1000, -1000, -1000, -1000, -2, 0, -11, -7,
	-1000, 25, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000,
}
var yyPgo = []int{

	0, 46, 18, 64, 65, 45, 44, 41, 40, 39,
	10,
}
var yyR1 = []int{

	0, 1, 1, 3, 3, 3, 3, 3, 3, 4,
	4, 5, 5, 5, 5, 5, 5, 6, 6, 6,
	7, 7, 8, 9, 9, 9, 9, 10, 10, 2,
	2, 2, 2, 2, 2, 2,
}
var yyR2 = []int{

	0, 1, 2, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	3, 3, 3, 3, 3, 3, 3, 1, 1, 3,
	3, 3, 1, 1, 1, 1,
}
var yyChk = []int{

	-1000, -1, -2, 29, -7, -8, -9, -10, -5, -6,
	23, 27, 28, 14, 15, 16, 17, 18, 19, 20,
	21, 22, -2, 13, 12, -2, -3, -4, 4, 5,
	6, 7, 8, 9, 10, 11, -3, -3, 4, -4,
	-2, -2, 30, 24, 26, 25, 25, 24, -10, 26,
}
var yyDef = []int{

	0, -2, 1, 0, 32, 33, 34, 35, 0, 0,
	0, 27, 28, 11, 12, 13, 14, 15, 16, 17,
	18, 19, 2, 0, 0, 0, 0, 0, 3, 4,
	5, 6, 7, 8, 9, 10, 0, 0, 3, 0,
	30, 31, 29, 20, 21, 22, 23, 24, 25, 26,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	29, 30,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 26, 27, 28,
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
	// 4 is TOKSTART above
	if c >= 4 && c-4 < len(yyToknames) {
		if yyToknames[c-4] != "" {
			return yyToknames[c-4]
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

	case 20:
		//line message_matcher_parser.y:115
		{
			//fmt.Println("string_test", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 21:
		//line message_matcher_parser.y:120
		{
			//fmt.Println("string_test regexp", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 22:
		//line message_matcher_parser.y:126
		{
			//fmt.Println("numeric_test", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 23:
		//line message_matcher_parser.y:132
		{
			//fmt.Println("field_test numeric", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 24:
		//line message_matcher_parser.y:137
		{
			//fmt.Println("field_test string", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 25:
		//line message_matcher_parser.y:142
		{
			//fmt.Println("field_test boolean", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 26:
		//line message_matcher_parser.y:147
		{
			//fmt.Println("field_test regexp", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{yyS[yypt-2], yyS[yypt-1], yyS[yypt-0]}})
		}
	case 29:
		//line message_matcher_parser.y:154
		{
			yyVAL = yyS[yypt-1]
		}
	case 30:
		//line message_matcher_parser.y:158
		{
			//fmt.Println("and", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-1]}})
		}
	case 31:
		//line message_matcher_parser.y:163
		{
			//fmt.Println("or", $1, $2, $3)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-1]}})
		}
	case 35:
		//line message_matcher_parser.y:171
		{
			//fmt.Println("boolean", $1)
			nodes = append(nodes, &tree{stmt: &Statement{op: yyS[yypt-0]}})
		}
	}
	goto yystack /* stack new state and value */
}
