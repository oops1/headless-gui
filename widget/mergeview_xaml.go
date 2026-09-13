package widget

import (
	"io"
	"path"
	"strconv"
	"strings"
)

// mergeview_xaml.go — тег <MergeView> в разметке.
//
//	<MergeView x:Name="merge" OursFile="ours.go" BaseFile="base.go" TheirsFile="theirs.go"
//	           ShowBase="True" SyntaxHighlight="True" ReadOnly="False"
//	           ConflictStyle="diff3" FontFamily="Consolas" FontSize="10"
//	           SaveCommand="{Binding Save}" ResolvedCommand="{Binding Left}"/>
//
// Три файла — редкий случай (обычно блоки приходят из приложения через
// SetChunks), но он делает разметку самодостаточной для демонстрации. Пути —
// относительно каталога разметки и не выходят за него, как у всех ресурсов
// (SEC-8). Разметка из fs.FS кладёт содержимое как текст: путей на диске у
// вкомпилированных файлов нет.

func buildXAMLMergeView(el xElement, baseDir string) Widget {
	m := NewMergeView(el.attr("FontFamily"), el.attr("HeaderFontFamily"))
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			m.SetFont("", "", v)
		}
	}
	flag := func(name string, set func(bool)) {
		if v := el.attr(name); v != "" {
			set(parseXAMLBool(v))
		}
	}
	flag("ShowBase", m.SetShowBase)
	flag("SyntaxHighlight", m.SetSyntaxHighlight)
	flag("ReadOnly", m.SetReadOnly)
	if v := el.attr("ConflictStyle"); v != "" {
		if strings.EqualFold(v, "diff3") {
			m.SetStyle(MergeStyleDiff3)
		} else {
			m.SetStyle(MergeStyleMerge)
		}
	}

	// MarkerSize — длина маркеров конфликта, как conflict-marker-size у git.
	if v := el.attr("MarkerSize"); v != "" {
		m.SetMarkerSize(xatoi(v))
	}

	base, ours, theirs := el.attr("BaseFile"), el.attr("OursFile"), el.attr("TheirsFile")
	if base != "" && ours != "" && theirs != "" {
		b, okB := readXAMLText(baseDir, base)
		o, okO := readXAMLText(baseDir, ours)
		t, okT := readXAMLText(baseDir, theirs)
		if okB && okO && okT {
			m.SetTexts(b, o, t)
			m.SetSides(
				MergeSideInfo{Title: Tr("merge.side.ours"), Note: xamlBase(ours)},
				MergeSideInfo{Title: Tr("merge.side.base"), Note: xamlBase(base)},
				MergeSideInfo{Title: Tr("merge.side.theirs"), Note: xamlBase(theirs)},
			)
		}
	}
	return m
}

// readXAMLText читает ресурс разметки как текст — из fs.FS или с диска.
// Ошибка (нет файла, путь за пределами каталога) оставляет контрол пустым:
// окно без слияния понятнее, чем окно, не открывшееся из-за одного файла.
func readXAMLText(baseDir, src string) (string, bool) {
	f, err := openXAMLResource(baseDir, src)
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func xamlBase(src string) string {
	return path.Base(strings.ReplaceAll(src, `\`, "/"))
}
