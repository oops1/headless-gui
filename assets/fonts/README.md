# Шрифты (assets/fonts)

Движок при старте (`engine.New`) **автоматически** регистрирует все файлы
`*.ttf` / `*.otf` из этой папки как именованные шрифты.

- Имя шрифта = имя файла без расширения, напр. `Roboto-Regular`.
- Для файла вида `Семейство-Regular.ttf` дополнительно создаётся псевдоним
  семейства: `Roboto-Regular.ttf` → доступен и как `Roboto`, и как `Roboto-Regular`.
- Если присутствует **Roboto-Regular.ttf**, он становится шрифтом **по умолчанию**
  (используется `DrawText`/всеми виджетами без явного `FontFamily`). Иначе
  по умолчанию — встроенный Go Regular.

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

Эти семейства уже лежат в папке (Regular/Bold/Italic/BoldItalic) с файлами лицензий:

| Семейство | XAML FontFamily | Лицензия | Файлы |
|-----------|-----------------|----------|-------|
| **Roboto** (по умолчанию) | `Roboto` | Apache-2.0 | `Roboto-Regular/Bold/Italic/BoldItalic.ttf` |
| **Open Sans** | `OpenSans` | Apache-2.0 | `OpenSans-Regular/Bold/Italic/BoldItalic.ttf` |
| **Inter** (оптич. размер 18pt) | `Inter` | SIL OFL-1.1 | `Inter-Regular/Bold/Italic/BoldItalic.ttf` |

Файлы лицензий: `Roboto-LICENSE.txt`, `OpenSans-LICENSE.txt`, `Inter-OFL.txt`.
