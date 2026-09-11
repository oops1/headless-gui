package diffview

import "unicode"

// TokenKind — вид лексемы подсветки.
type TokenKind uint8

const (
	TokenPlain TokenKind = iota
	TokenKeyword
	TokenString
	TokenComment
	TokenNumber
	TokenFunc
)

// Token — участок строки [Start, End) в рунах и его вид.
type Token struct {
	Start, End int
	Kind       TokenKind
}

// keywords — ключевые слова популярных C-подобных языков, Python и Go вместе.
// Подсветка грубая намеренно: она отличает код от комментариев и строк, а не
// разбирает язык — разбора нет, как нет и состояния между строками.
var keywords = map[string]bool{
	"package": true, "import": true, "func": true, "type": true, "struct": true,
	"interface": true, "return": true, "if": true, "else": true, "for": true,
	"range": true, "switch": true, "case": true, "default": true, "const": true,
	"var": true, "map": true, "chan": true, "go": true, "defer": true,
	"break": true, "continue": true, "nil": true, "true": true, "false": true,
	"string": true, "bool": true, "int": true, "error": true, "byte": true,
	"class": true, "public": true, "private": true, "static": true, "void": true,
	"def": true, "let": true, "function": true, "new": true, "this": true,
	"self": true, "None": true, "True": true, "False": true, "from": true,
	"in": true, "while": true, "try": true, "catch": true, "finally": true,
	"throw": true, "async": true, "await": true, "using": true, "namespace": true,
}

func isIdent(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// Tokenize размечает строку для подсветки: ключевые слова, строки, числа,
// вызовы функций и комментарии до конца строки («//» и «#»).
func Tokenize(rs []rune) []Token {
	var out []Token
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case r == '/' && i+1 < len(rs) && rs[i+1] == '/', r == '#':
			out = append(out, Token{i, len(rs), TokenComment})
			return out
		case r == '"' || r == '\'' || r == '`':
			j := i + 1
			for j < len(rs) && rs[j] != r {
				if rs[j] == '\\' && r != '`' {
					j++
				}
				j++
			}
			j = min(j+1, len(rs))
			out = append(out, Token{i, j, TokenString})
			i = j
		case unicode.IsDigit(r):
			j := i + 1
			for j < len(rs) && (isIdent(rs[j]) || rs[j] == '.') {
				j++
			}
			out = append(out, Token{i, j, TokenNumber})
			i = j
		case isIdent(r):
			j := i + 1
			for j < len(rs) && isIdent(rs[j]) {
				j++
			}
			k := TokenPlain
			if keywords[string(rs[i:j])] {
				k = TokenKeyword
			} else if j < len(rs) && rs[j] == '(' {
				k = TokenFunc
			}
			out = append(out, Token{i, j, k})
			i = j
		default:
			j := i + 1
			for j < len(rs) && !isIdent(rs[j]) && rs[j] != '"' && rs[j] != '\'' && rs[j] != '`' && rs[j] != '/' && rs[j] != '#' {
				j++
			}
			out = append(out, Token{i, j, TokenPlain})
			i = j
		}
	}
	return out
}
