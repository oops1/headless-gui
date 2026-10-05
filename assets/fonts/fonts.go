// Package fonts — шрифты движка, вшитые в исполняемый файл.
//
// Каталог assets/fonts, который движок читает при старте, ищется относительно
// рабочего каталога процесса. Программа в чужом модуле, установленная в
// Program Files или запущенная службой, его не видит, а go:embed не умеет
// заглядывать в родительские каталоги — своей копии у потребителя не
// получится. Поэтому вшитые файлы лежат здесь, рядом с самими шрифтами, и
// отдаются как fs.FS.
//
// Вшито только то, что нужно оболочке в стиле Windows 10: Open Sans
// (Light, Regular, SemiBold, Bold и курсивы, около 0,8 МБ). Остальные
// шрифты каталога (Roboto, Inter, DejaVu…) остаются файлами: тащить ради
// них 9 МБ в каждую программу незачем. Пакет подключается явно, поэтому
// программы, которым он не нужен, не платят за него размером.
//
//	import "github.com/oops1/headless-gui/v3/assets/fonts"
//
//	if err := fonts.Register(eng); err != nil { ... }
//	eng.SetDefaultFont("OpenSans")
//
// После этого тема может ссылаться на семейство по названию —
// theme.FontSpec{Family: "Open Sans", Weight: theme.WeightSemiBold}.
// Лицензия SIL OFL-1.1 лежит рядом (OpenSans-OFL.txt) и входит в FS: при
// распространении она обязана ехать со шрифтом.
package fonts

import (
	"embed"
	"io/fs"
)

// FS — вшитые файлы шрифтов и лицензия; корень FS — каталог со шрифтами.
//
//go:embed OpenSans-*.ttf OpenSans-OFL.txt
var FS embed.FS

// OpenSans — название семейства Open Sans для FontSpec.Family и XAML.
const OpenSans = "Open Sans"

// Registrar — то, что умеет принимать шрифты из fs.FS. Реализует *engine.Engine;
// интерфейс избавляет пакет шрифтов от зависимости на движок.
type Registrar interface {
	RegisterFontFS(fsys fs.FS, dir string) error
}

// Register регистрирует вшитые шрифты в движке (до Start). Имена те же, что
// у каталога на диске: «OpenSans-Regular» и «OpenSans», «OpenSans-SemiBold»,
// «OpenSans-Light» и т. д.; семейство доступно и как «Open Sans».
//
// Шрифт по умолчанию не меняется: SetDefaultFont("OpenSans") потребитель
// зовёт сам, если хочет, чтобы Open Sans писала весь интерфейс.
func Register(r Registrar) error {
	return r.RegisterFontFS(FS, ".")
}
