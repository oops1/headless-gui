// Package clipboard — системный буфер обмена по ОС (ADR-0012, 8.7): Windows
// (текст, HTML, картинки, файлы и виртуальные файлы через COM), macOS
// (pbcopy/pbpaste), Linux (xclip/xsel, иначе — в памяти). Переносимая часть —
// интерфейсы провайдера, буфер в памяти, CF_HTML, списки файлов — в
// widget/base.
//
// Пакет нужен ради регистрации: при загрузке он ставит провайдер своей ОС
// (base.SetClipboardProvider). Его импортирует фасад widget, так что буфер
// есть у всех программ на widget; программа на widget/base импортирует его
// сама:
//
//	import _ "github.com/oops1/headless-gui/v3/platform/clipboard"
//
// На ОС без реализации пакет пуст, и остаётся буфер в памяти.
//
// Свой провайдер ставьте вызовом widget.SetClipboardProvider во время работы
// (в main): в init пакета, который импортирует только widget/base, он может
// выполниться раньше init этого пакета и быть перекрыт системным.
package clipboard
