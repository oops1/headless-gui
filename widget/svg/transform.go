package svg

import (
	"math"
	"strconv"
	"strings"
)

// ParseTransform разбирает значение атрибута transform: последовательность
// функций translate/scale/rotate/matrix/skewX/skewY, применяемых слева
// направо. Возвращает единичную матрицу для пустой/неразобранной строки.
//
// Порядок: transform="A B" эквивалентно A.Mul(B) (сначала A к системе
// координат, затем B) — точка отображается A.Apply(B.Apply(p)).
func ParseTransform(s string) Matrix {
	m := Identity()
	i := 0
	n := len(s)
	for i < n {
		// имя функции
		for i < n && (s[i] == ' ' || s[i] == ',' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		start := i
		for i < n && s[i] != '(' {
			i++
		}
		if i >= n {
			break
		}
		name := strings.TrimSpace(s[start:i])
		i++ // пропустить '('
		argStart := i
		for i < n && s[i] != ')' {
			i++
		}
		if i > n {
			break
		}
		args := parseFloats(s[argStart:i])
		if i < n {
			i++ // пропустить ')'
		}
		m = m.Mul(transformFunc(name, args))
	}
	return m
}

func transformFunc(name string, a []float64) Matrix {
	switch name {
	case "translate":
		if len(a) >= 2 {
			return Translate(a[0], a[1])
		}
		if len(a) == 1 {
			return Translate(a[0], 0)
		}
	case "scale":
		if len(a) >= 2 {
			return ScaleM(a[0], a[1])
		}
		if len(a) == 1 {
			return ScaleM(a[0], a[0])
		}
	case "rotate":
		if len(a) >= 3 {
			return RotateAboutDeg(a[0], a[1], a[2])
		}
		if len(a) >= 1 {
			return RotateDeg(a[0])
		}
	case "matrix":
		if len(a) >= 6 {
			return Matrix{A: a[0], B: a[1], C: a[2], D: a[3], E: a[4], F: a[5]}
		}
	case "skewX":
		if len(a) >= 1 {
			return Matrix{A: 1, D: 1, C: tanDeg(a[0])}
		}
	case "skewY":
		if len(a) >= 1 {
			return Matrix{A: 1, D: 1, B: tanDeg(a[0])}
		}
	}
	return Identity()
}

func parseFloats(s string) []float64 {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	var out []float64
	for _, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

func tanDeg(deg float64) float64 {
	return math.Tan(deg * math.Pi / 180)
}
