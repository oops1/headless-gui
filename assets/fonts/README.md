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
путём.

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
| **Open Sans** | `OpenSans` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `OpenSans-OFL.txt` |
| **Inter** (оптич. размер 18pt) | `Inter` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Inter-OFL.txt` |
| **Liberation Sans** | `LiberationSans` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Liberation-OFL.txt` |
| **Liberation Mono** | `LiberationMono` | SIL OFL-1.1 | Regular/Bold/Italic/BoldItalic | `Liberation-OFL.txt` |
| **DejaVu Sans** | `DejaVuSans` | Bitstream Vera + PD | Regular/Bold/Oblique/BoldOblique | `DejaVu-LICENSE.txt` |
| **DejaVu Sans Mono** | `DejaVuSansMono` | Bitstream Vera + PD | Regular/Bold/Oblique/BoldOblique | `DejaVu-LICENSE.txt` |
| **Go Regular** | `Go-Regular` | BSD-3-Clause | Regular | `Go-LICENSE.txt` |

Что чем закрывается:

- **Roboto, Open Sans, Inter** — интерфейсные гротески, ими рисуются виджеты.
- **Liberation Sans и Mono** — метрически совместимы с Arial и Courier New:
  та же ширина строки при том же кегле. Нужны там, где макет пришёл из Windows
  и должен совпасть по ширине, а не только по начертанию.
- **DejaVu Sans и Mono** — самое широкое покрытие символов из здешних:
  кириллица, греческий, псевдографика, стрелки, ✓ ✗ ⚠. Движок берёт
  `DejaVuSans.ttf` и `DejaVuSansMono.ttf` из этой папки ещё и как **fallback**
  к основному шрифту, если системных шрифтов на машине не нашлось
  (см. `assetFallbackFontPaths` в `engine/font.go`). Оттого их имена менять нельзя.

Начертания Bold/Italic здесь — самостоятельные именованные шрифты, а не
варианты веса: `FontWeight`/`FontStyle` переключают только встроенные Go-шрифты.
Жирный Liberation берётся как `FontFamily="LiberationSans-Bold"`.

## Обязанности при распространении

- **SIL OFL-1.1**: сохранять файл лицензии; шрифт нельзя продавать сам по себе
  (в составе продукта — можно); при изменении файла шрифта менять имя семейства,
  если оно объявлено Reserved Font Name (у Liberation это «Liberation»);
  производное остаётся под OFL.
- **Bitstream Vera**: сохранять копирайт и текст разрешения; изменённый шрифт
  не должен содержать в имени «Bitstream» или «Vera».
- **BSD-3-Clause**: сохранять копирайт и текст лицензии.
