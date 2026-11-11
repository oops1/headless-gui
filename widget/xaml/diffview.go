package xaml

import (
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// diffview.go — тег <DiffView> в разметке.
//
//	<DiffView x:Name="diff" LeftFile="old/config.go" RightFile="new/config.go"
//	          HideUnchanged="True" ContextLines="3" IgnoreWhitespace="False"
//	          SyntaxHighlight="True" ReadOnlyLeft="True" WatchFiles="True"
//	          FontFamily="Consolas" FontSize="10"
//	          CollapsedSide="None" ShowCollapseButtons="True" Split="0.5"
//	          SaveCommand="{Binding Save}" DiffChangedCommand="{Binding Count}"/>
//
// Пути файлов — относительно каталога разметки и не выходят за него, как у
// всех ресурсов (SEC-8): разметка из чужих рук не должна открывать ей файлы по
// всему диску. Разметка из fs.FS (LoadUIFromFS) кладёт содержимое как текст без
// файла — сохранять его приложение будет через SaveAs, путей на диске у
// вкомпилированных файлов нет.

func buildXAMLDiffView(el Element, baseDir string) base.Widget {
	d := base.NewDiffView(el.Attr("FontFamily"), el.Attr("HeaderFontFamily"))
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			d.SetFont("", "", v)
		}
	}
	flag := func(name string, set func(bool)) {
		if v := el.Attr(name); v != "" {
			set(parseXAMLBool(v))
		}
	}
	flag("HideUnchanged", d.SetHideUnchanged)
	flag("IgnoreWhitespace", d.SetIgnoreWhitespace)
	flag("SyntaxHighlight", d.SetSyntaxHighlight)
	flag("ShowHeaders", d.SetShowHeaders)
	flag("ShowReadOnlyMark", d.SetShowReadOnlyMark)
	flag("ReadOnlyLeft", func(v bool) { d.SetReadOnly(base.DiffLeft, v) })
	flag("ReadOnlyRight", func(v bool) { d.SetReadOnly(base.DiffRight, v) })
	flag("ShowCollapseButtons", d.SetShowCollapseButtons)
	if v := el.Attr("ContextLines"); v != "" {
		d.SetContextLines(ParseInt(v))
	}
	// Свёрнутая сторона и доля ширины (GG-95): CollapsedSide="None|Left|Right",
	// Split="0.6" — доля левой стороны.
	switch strings.ToLower(strings.TrimSpace(el.Attr("CollapsedSide"))) {
	case "left":
		d.SetCollapsed(base.DiffLeft, true)
	case "right":
		d.SetCollapsed(base.DiffRight, true)
	}
	if v := el.Attr("Split"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			d.SetSplit(f)
		}
	}
	for side, attr := range map[base.DiffSide]string{base.DiffLeft: "LeftFile", base.DiffRight: "RightFile"} {
		if src := el.Attr(attr); src != "" {
			loadDiffXAMLFile(d, side, baseDir, src)
		}
	}
	// Слежение включается после загрузки: иначе первый же опрос застал бы
	// сторону без отметки времени.
	flag("WatchFiles", d.SetWatchFiles)
	return d
}

// loadDiffXAMLFile загружает сторону из файла разметки. Ошибка (нет файла,
// путь за пределами каталога, двоичный файл) оставляет сторону пустой — окно
// с пустым сравнением понятнее, чем окно, не открывшееся из-за одного файла.
func loadDiffXAMLFile(d *base.DiffView, side base.DiffSide, baseDir, src string) {
	if base.IsResourceFS(baseDir) {
		f, err := base.OpenResource(baseDir, src)
		if err != nil {
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return
		}
		d.SetText(side, path.Base(strings.ReplaceAll(src, `\`, "/")), "", string(data))
		return
	}
	full, err := base.ResolveResource(baseDir, src)
	if err != nil {
		return
	}
	_ = d.LoadFile(side, full)
}
