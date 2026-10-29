package base

// resources.go — ресурсы разметки, не зависящие от XAML (ADR-0012, 8.2b):
// граница путей ресурсов (SEC-8), реестр fs.FS для LoadUIFromXAMLFS,
// единая точка открытия ресурса и декодирование картинок с пределами.
// Ими пользуются и XAML-загрузчик (widget/xaml), и ядро (IconSet,
// ImageWidget).

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ErrResourceOutsideBase — путь ресурса выводит за пределы каталога XAML-файла.
var ErrResourceOutsideBase = errors.New("xaml: путь ресурса выходит за пределы каталога разметки")

// xamlResourceLogged — уже залогированные отклонённые пути (чтобы разметка,
// повторяющая один и тот же плохой путь в сотне элементов, не залила журнал).
var xamlResourceLogged sync.Map

// ResolveResource приводит путь ресурса из XAML-атрибута (Image Source,
// Button IconSource, Panel BackgroundImage, SVGIcon Source, TrayIcon) к пути
// на диске и проверяет, что чтение остаётся в дозволенных границах.
//
// Политика (SEC-8):
//
//   - baseDir ЗАДАН — разметка пришла ИЗ ФАЙЛА. Такой XAML нередко приходит
//     извне (тема, плагин, скачанный макет), поэтому он трактуется как данные,
//     а не как код: путь считается относительным от каталога XAML-файла и
//     ОБЯЗАН остаться внутри него. Абсолютные пути («C:\Windows\…», «/etc/…»)
//     и выходы наружу («..\..\secret.png») отклоняются — иначе один атрибут
//     Source= давал бы чтение любого файла, доступного процессу.
//
//   - baseDir ПУСТ — разметка передана строкой из кода (LoadUIFromXAML,
//     LoadUIFromXAMLWithContext). Строку сформировал сам программист, границы
//     задавать не от кого и незачем: относительный путь резолвится от текущего
//     каталога, абсолютный разрешён — ровно как при прямом os.Open в его коде.
//
// Символические ссылки намеренно не разыменовываются: путь проверяется
// лексически (после filepath.Clean). Для дерева ресурсов приложения этого
// достаточно, а EvalSymlinks на каждую иконку — лишний поход в ФС.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func ResolveResource(baseDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("xaml: пустой путь ресурса")
	}
	if baseDir == "" {
		// Загрузка из строки — доверяем вызывающему коду (см. комментарий выше).
		return filepath.Clean(src), nil
	}
	// Абсолютный путь, корневой («/x», «\x») или с указанием тома («C:x»)
	// уводит из baseDir ещё до Join — отклоняем сразу.
	if filepath.IsAbs(src) || strings.HasPrefix(src, "/") || strings.HasPrefix(src, `\`) ||
		filepath.VolumeName(src) != "" {
		err := fmt.Errorf("%w: абсолютный путь %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, err)
		return "", err
	}
	full := filepath.Join(baseDir, src) // Join уже делает Clean
	rel, err := filepath.Rel(baseDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rerr := fmt.Errorf("%w: %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, rerr)
		return "", rerr
	}
	return full, nil
}

// logResourceRejected пишет в журнал один раз на каждый отклонённый путь.
func logResourceRejected(src string, err error) {
	if _, seen := xamlResourceLogged.LoadOrStore(src, struct{}{}); seen {
		return
	}
	log.Printf("xaml: ресурс отклонён: %v", err)
}

const (
	// MaxImageFileBytes — предельный размер файла изображения. Больше этого
	// не читаем вовсе: PNG/JPEG таких размеров в UI не бывает, а вот «файл»
	// из /dev/zero или сетевой шары может быть бесконечным.
	MaxImageFileBytes = 256 << 20 // 256 МБ

	// defaultMaxImagePixels — предел площади изображения по умолчанию.
	// 64 Мпикс в RGBA — это 256 МБ на буфер; заголовок PNG допускает
	// 65535×65535 (≈4.3 Гпикс, 16 ГБ при декодировании) при считанных
	// килобайтах самого файла — классическая декомпрессионная бомба.
	defaultMaxImagePixels = 64 << 20 // 64 Мпикс
)

// maxImagePixels — текущий предел площади (атомарно: настраивается из кода
// приложения, читается из потоков отрисовки/загрузки).
var maxImagePixels atomic.Int64

func init() { maxImagePixels.Store(defaultMaxImagePixels) }

// MaxImagePixels возвращает текущий предел площади декодируемого изображения
// (в пикселях).
func MaxImagePixels() int64 { return maxImagePixels.Load() }

// SetMaxImagePixels меняет предел площади декодируемого изображения.
// Значение ≤ 0 возвращает предел по умолчанию (64 Мпикс).
func SetMaxImagePixels(n int64) {
	if n <= 0 {
		n = defaultMaxImagePixels
	}
	maxImagePixels.Store(n)
}

// checkImageBounds проверяет заявленные в заголовке размеры изображения.
func checkImageBounds(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("некорректный размер изображения %dx%d", w, h)
	}
	limit := MaxImagePixels()
	if px := int64(w) * int64(h); px > limit {
		return fmt.Errorf("изображение %dx%d (%d пикс.) превышает предел %d пикс.", w, h, px, limit)
	}
	return nil
}

// DecodeImageBounded декодирует изображение из r с защитой от
// декомпрессионной бомбы (SEC-9).
//
// Сначала читается ТОЛЬКО заголовок (image.DecodeConfig) и проверяется
// заявленная площадь: если она превышает MaxImagePixels, декодирование не
// начинается вовсе — то есть память под гигабайтный растр не выделяется.
// Дополнительно поток ограничен MaxImageFileBytes.
//
// Экспортирована, чтобы движок (engine.SetBackgroundFile) читал изображения
// через ту же проверку: engine импортирует widget, обратное невозможно.
func DecodeImageBounded(r io.Reader) (image.Image, error) {
	if rs, ok := r.(io.ReadSeeker); ok {
		// Файл: читаем заголовок и перематываемся — без буферизации всего
		// содержимого в память.
		cfg, _, err := image.DecodeConfig(bufio.NewReader(io.LimitReader(rs, MaxImageFileBytes)))
		if err != nil {
			return nil, err
		}
		if err := checkImageBounds(cfg.Width, cfg.Height); err != nil {
			return nil, err
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bufio.NewReader(io.LimitReader(rs, MaxImageFileBytes)))
		return img, err
	}

	// Поток без перемотки — буферизуем, но не больше предела.
	data, err := io.ReadAll(io.LimitReader(r, MaxImageFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxImageFileBytes {
		return nil, fmt.Errorf("изображение больше предела %d байт", int64(MaxImageFileBytes))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkImageBounds(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// LoadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
// Экспортирован для использования в приложениях (иконки TreeView, и т.д.).
func LoadImageFile(path string) (*image.RGBA, error) {
	return loadImageFile(path)
}

// loadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
// Размер файла и площадь изображения ограничены (см. DecodeImageBounded).
func loadImageFile(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// image.Decode использует зарегистрированные декодеры (png, jpeg);
	// перед ним проверяется заголовок — защита от бомбы (SEC-9).
	img, err := DecodeImageBounded(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	// Конвертируем в RGBA
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}

// xamlFSMarker — префикс синтетического baseDir. Ведущий NUL-байт не может
// встретиться в filepath.Dir(реального XAML-файла), поэтому коллизия с живым
// диском исключена без доп. проверок.
const xamlFSMarker = "\x00xamlfs:"

var (
	xamlFSRegistry    sync.Map // string(маркер+id) → fs.FS
	xamlFSRegistrySeq atomic.Uint64
)

// RegisterResourceFS заводит новый синтетический baseDir для fsys.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func RegisterResourceFS(fsys fs.FS) string {
	id := xamlFSRegistrySeq.Add(1)
	key := xamlFSMarker + strconv.FormatUint(id, 36)
	xamlFSRegistry.Store(key, fsys)
	return key
}

// UnregisterResourceFS убирает запись реестра (см. LoadUIFromXAMLFS: у неё нет
// DataContext, поэтому запись не нужна дольше одной синхронной загрузки).
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func UnregisterResourceFS(key string) {
	xamlFSRegistry.Delete(key)
}

// IsResourceFS сообщает, что baseDir — синтетический каталог fs.FS из
// RegisterResourceFS, а не путь на диске.
//
//facadegen:skip
func IsResourceFS(baseDir string) bool {
	_, ok := xamlFSRegistry.Load(baseDir)
	return ok
}

// ResolveResourceFS приводит путь ресурса из атрибута разметки к виду,
// который принимает fs.FS: разделитель всегда "/", без ведущего "/" (таково
// требование io/fs.ValidPath). Разметку часто пишут на Windows, поэтому "\"
// сначала заменяется на "/" — это единственное отличие от ResolveResource
// (диск, выше); политика границы та же (SEC-8): абсолютный путь и
// выход за пределы корня отклоняются с ErrResourceOutsideBase. fs.FS и так не
// откроет файл вне себя (Open вернёт ошибку), но сообщение должно быть
// понятным, а не «invalid name» из stdlib.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func ResolveResourceFS(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("xaml: пустой путь ресурса")
	}
	slashed := strings.ReplaceAll(src, `\`, "/")
	if strings.HasPrefix(slashed, "/") || filepath.VolumeName(src) != "" {
		err := fmt.Errorf("%w: абсолютный путь %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, err)
		return "", err
	}
	clean := path.Clean(slashed)
	if clean == ".." || strings.HasPrefix(clean, "../") || !fs.ValidPath(clean) {
		err := fmt.Errorf("%w: %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, err)
		return "", err
	}
	return clean, nil
}

// OpenResource — ЕДИНАЯ точка чтения ресурса разметки (Image Source,
// Button Icon/IconSource, Panel BackgroundImage, SVGIcon Source, TrayIcon):
// решает, откуда читать (диск или fs.FS из LoadUIFromXAMLFS), и открывает файл.
// Дальше вызывающему коду (декодирование PNG/JPEG/SVG) источник уже не важен.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func OpenResource(baseDir, src string) (io.ReadCloser, error) {
	if v, ok := xamlFSRegistry.Load(baseDir); ok {
		p, err := ResolveResourceFS(src)
		if err != nil {
			return nil, err
		}
		return v.(fs.FS).Open(p)
	}
	full, err := ResolveResource(baseDir, src)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}
