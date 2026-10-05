# Шрифты (assets/fonts)

Движок при старте (`engine.New`) **автоматически** регистрирует все файлы
`*.ttf` / `*.otf` из этой папки как именованные шрифты.

- Имя шрифта = имя файла без расширения, напр. `Roboto-Regular`.
- Для файла вида `Семейство-Regular.ttf` дополнительно создаётся псевдоним
  семейства: `Roboto-Regular.ttf` → доступен и как `Roboto`, и как `Roboto-Regular`.
- Если присутствует **Roboto-Regular.ttf**, он становится шрифтом **по умолчанию**
  (используется `DrawText`/всеми виджетами без явного `FontFamily`). Иначе
  по умолчанию — встроенный Go Regular.

Папка ищется **относительно рабочего каталога процесса**. Программа, которая
запускается не из корня репозитория, этих шрифтов не увидит: ей нужно либо
положить свою копию рядом с собой, либо позвать `RegisterFontDir` с абсолютным
путём, либо вшить шрифты в исполняемый файл (следующий раздел).

## Вшитые шрифты (go:embed)

`go:embed` не видит родительские каталоги, поэтому потребитель из другого
модуля не может вшить эту папку у себя. Для этого рядом с файлами лежит пакет
`github.com/oops1/headless-gui/v3/assets/fonts` — в нём вшит **только Open Sans**
(Light, Regular, SemiBold, Bold, Italic, BoldItalic и лицензия, ~0,8 МБ).
Остальные шрифты остаются файлами: тащить 9 МБ в каждую программу незачем.
Пакет подключается явно — программам, которым он не нужен, он размера не
прибавляет.

```go
import "github.com/oops1/headless-gui/v3/assets/fonts"

eng := engine.New(1280, 800, 30)
if err := fonts.Register(eng); err != nil { /* опечатка в embed */ }
eng.SetDefaultFont("OpenSans") // по желанию
```

`fonts.FS` — обычный `fs.FS`, его можно отдать и в `eng.RegisterFontFS`.
Существующие программы, которые ищут `assets/fonts` в рабочем каталоге, не
меняются: файлы на месте, автозагрузка работает как раньше.

## Семейство, вес, наклон

Помимо имени файла (`OpenSans-SemiBold`) шрифт выбирается по **семейству, весу и
наклону**. Каждый зарегистрированный шрифт записывается в таблицу семейств по
собственным данным (name-таблица и `OS/2.usWeightClass` из файла, имя файла —
запасной признак). Название семейства сравнивается без регистра, пробелов и
дефисов: `Open Sans`, `OpenSans`, `open-sans` — одно и то же.

```go
// Код: имя шрифта для DrawTextFont / MeasureTextFont / Label.FontName.
name := widget.FontFace("Open Sans", widget.FontWeightSemiBold, false)
ctx.DrawTextFont("Заголовок", x, y, 8.5, name, col)

// Тема: desktop/paint.go учитывает Weight, Bold и Italic из FontSpec.
theme.FontSpec{Family: "Open Sans", Size: 8.5, Weight: theme.WeightLight}
```

Подбор — по правилам CSS Fonts: нужного веса нет — берётся ближайший (для 500 —
сначала 400, для лёгкого — более лёгкий, для тяжёлого — более тяжёлый); наклон
важнее веса. Пустое семейство — «шрифт по умолчанию нужного веса»: если по
умолчанию стоит Open Sans, жирный даст `OpenSans-Bold`, а если встроенный Go
Regular — встроенный Go Bold. Кегль может быть дробным (`8.5`).

## Использование

XAML:

```xml
<TextBlock Text="Привет" FontFamily="Roboto"/>
<TextBlock Text="Заголовок" FontFamily="Inter" FontSize="20"/>
```

Код:

```go
eng := engine.New(1280, 800, 30)
eng.SetDefaultFont("Inter")       // сменить шрифт по умолчанию
eng.RegisterFontDir("my/fonts")   // подхватить ещё каталог
fmt.Println(eng.AvailableFonts()) // список зарегистрированных
// одиночный файл:
eng.RegisterFontFile("Roboto", "assets/fonts/Roboto-Regular.ttf")
```

## Включённые свободные шрифты

Все семейства здесь свободны и разрешают распространение в составе продукта,
включая коммерческий. Лицензия каждого лежит рядом отдельным файлом — он
обязан уехать вместе со шрифтом.

| Семейство | XAML FontFamily | Лицензия | Начертания | Файл лицензии |
|-----------|-----------------|----------|------------|---------------|
| **Roboto** (по умолчанию) | `Roboto` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Roboto-OFL.txt` |
| **Open Sans** | `OpenSans`, `Open Sans` | SIL OFL-1.1 | Light/Regular/SemiBold/Bold/Italic/BoldItalic | `OpenSans-OFL.txt` |
| **Inter** (оптич. размер 18pt) | `Inter` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Inter-OFL.txt` |
| **Liberation Sans** | `LiberationSans` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Liberation-OFL.txt` |
| **Liberation Mono** | `LiberationMono` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Liberation-OFL.txt` |
| **DejaVu Sans** | `DejaVuSans` | Bitstream Vera + PD | Regular/Bold/Oblique/BoldOblique | `DejaVu-LICENSE.txt` |
| **DejaVu Sans Mono** | `DejaVuSansMono` | Bitstream Vera + PD | Regular/Bold/Oblique/BoldOblique | `DejaVu-LICENSE.txt` |
| **Golos Text** | `GolosText` | SIL OFL-1.1 | Regular/Medium/SemiBold/Bold/ExtraBold/Black | `GolosText-OFL.txt` |
| **Go Regular** | `Go-Regular` | BSD-3-Clause | Regular | `Go-LICENSE.txt` |

Что чем закрывается:

- **Roboto, Open Sans, Inter** — интерфейсные гротески, ими рисуются виджеты.
- **Golos Text** — гротеск с полной кириллицей и шестью весами: единственное
  здешнее семейство, где вес выбирается начертанием от Regular до Black
  (`FontFamily="GolosText-SemiBold"`), а не только Regular/Bold.
- **Liberation Sans и Mono** — метрически совместимы с Arial и Courier New:
  та же ширина строки при том же кегле. Нужны там, где макет пришёл из Windows
  и должен совпасть по ширине, а не только по начертанию.
- **DejaVu Sans и Mono** — самое широкое покрытие символов из здешних:
  кириллица, греческий, псевдографика, стрелки, ✓ ✗ ⚠. Движок берёт
  `DejaVuSans.ttf` и `DejaVuSansMono.ttf` из этой папки ещё и как **fallback**
  к основному шрифту, если системных шрифтов на машине не нашлось
  (см. `assetFallbackFontPaths` в `engine/font.go`). Оттого их имена менять нельзя.

Начертания Bold/Italic здесь — самостоятельные именованные шрифты: XAML
`FontWeight`/`FontStyle` переключают только встроенные Go-шрифты, жирный
Liberation берётся как `FontFamily="LiberationSans-Bold"`. Выбор «семейство +
вес + наклон» (раздел выше) работает поверх этих файлов: `widget.FontFace`,
`theme.FontSpec.Weight`.

## Обязанности при распространении

- **SIL OFL-1.1**: сохранять файл лицензии; шрифт нельзя продавать сам по себе
  (в составе продукта — можно); при изменении файла шрифта менять имя семейства,
  если оно объявлено Reserved Font Name (у Liberation это «Liberation»);
  производное остаётся под OFL.
- **Bitstream Vera**: сохранять копирайт и текст разрешения; изменённый шрифт
  не должен содержать в имени «Bitstream» или «Vera».
- **BSD-3-Clause**: сохранять копирайт и текст лицензии.
