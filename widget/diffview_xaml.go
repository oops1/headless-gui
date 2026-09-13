package widget

import (
	"io"
	"path"
	"strconv"
	"strings"
)

// diffview_xaml.go — тег <DiffView> в разметке.
//
//	<DiffView x:Name="diff" LeftFile="old/config.go" RightFile="new/config.go"
//	          HideUnchanged="True" ContextLines="3" IgnoreWhitespace="False"
//	          SyntaxHighlight="True" ReadOnlyLeft="True" WatchFiles="True"
//	          FontFamily="Consolas" FontSize="10"
//	          SaveCommand="{Binding Save}" DiffChangedCommand="{Binding Count}"/>
//
// Пути файлов — относительно каталога разметки и не выходят за него, как у
// всех ресурсов (SEC-8): разметка из чужих рук не должна открывать ей файлы по
// всему диску. Разметка из fs.FS (LoadUIFromFS) кладёт содержимое как текст без
// файла — сохранять его приложение будет через SaveAs, путей на диске у
// вкомпилированных файлов нет.

func buildXAMLDiffView(el xElement, baseDir string) Widget {
	d := NewDiffView(el.attr("FontFamily"), el.attr("HeaderFontFamily"))
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			d.SetFont("", "", v)
		}
	}
	flag := func(name string, set func(bool)) {
		if v := el.attr(name); v != "" {
			set(parseXAMLBool(v))
		}
	}
	flag("HideUnchanged", d.SetHideUnchanged)
	flag("IgnoreWhitespace", d.SetIgnoreWhitespace)
	flag("SyntaxHighlight", d.SetSyntaxHighlight)
	flag("ShowHeaders", d.SetShowHeaders)
	flag("ShowReadOnlyMark", d.SetShowReadOnlyMark)
	flag("ReadOnlyLeft", func(v bool) { d.SetReadOnly(DiffLeft, v) })
	flag("ReadOnlyRight", func(v bool) { d.SetReadOnly(DiffRight, v) })
	if v := el.attr("ContextLines"); v != "" {
		d.SetContextLines(xatoi(v))
	}
	for side, attr := range map[DiffSide]string{DiffLeft: "LeftFile", DiffRight: "RightFile"} {
		if src := el.attr(attr); src != "" {
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
func loadDiffXAMLFile(d *DiffView, side DiffSide, baseDir, src string) {
	if _, isFS := xamlFSRegistry.Load(baseDir); isFS {
		f, err := openXAMLResource(baseDir, src)
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
	full, err := resolveXAMLResource(baseDir, src)
	if err != nil {
		return
	}
	_ = d.LoadFile(side, full)
}
