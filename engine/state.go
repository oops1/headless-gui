package engine

// state.go — состояние движка по областям (ADR-0011, задача 7.1).
//
// Поля Engine разнесены по областям: сцена (холст, дерево, сборка кадра),
// цикл (горутина кадров, очередь Post, таймеры, темп и доставка), оверлеи
// (модальные окна, хостовые попапы, подсказка, переходы) — встроенные
// структуры ниже; встраивание сохраняет имена: e.canvas, e.frameMu читаются
// как прежде. Ввод (фокус, захват, касания, курсор) вынесен в core/input
// (7.2): поле e.in, адаптер — input.go. Этапы 7.3–7.4 так же выносят цикл и
// оверлеи в core/loop и core/overlay; здесь закреплён их состав и порядок
// замков.
//
// Порядок замков (подробно — ADR-0011, проверка — lockorder_test.go):
//
//   - frameMu — единственный замок, под которым берутся другие: кадр
//     (draw + diff) и структурные операции над холстом держат его, а всё,
//     что зовут под ним (Draw виджетов, хосты, слои), берёт только листовые
//     замки.
//   - Остальные замки — листовые: под ними не берут ни другой замок, ни
//     frameMu, и не зовут код виджетов и потребителя. Секции короткие:
//     снять указатель или очередь, отпустить, работать вне замка.
//   - e.mu (указатели холста и корня) берётся под frameMu в SetScale,
//     SetResolution, SetTheme и в кадре; обратного порядка нет: под e.mu не
//     зовут ни SetBounds корня (он заявляет повреждение через реестр), ни
//     кадр.
//
// Горутина-владелец указана у каждой группы: «только горутина кадра» значит,
// что поле читают и пишут под frameMu или из renderFrame, и замка ему не
// нужно; «поток-источник ввода» — горутина, зовущая Send* (под window.Run это
// та же горутина кадра).

import (
	"image"
	"sync"
	"sync/atomic"
	"time"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// sceneState — холст, дерево виджетов и сборка кадра: путь команд, тайловый
// растр, хэш-дифф, один буфер, внешние поверхности, службы дерева (замер
// текста, SVG, переносы). Будущий остаток пакета engine после выноса
// областей.
type sceneState struct {
	canvas    *Canvas
	fontCache *FontCache
	imgInv    *hgraster.ImageInvalidator // журнал сбросов кэша изображений всех холстов движка (imagecache.go)
	bgSrc     image.Image                // исходный фон (до масштабирования); нужен при SetResolution
	root      widget.Widget
	mu        sync.RWMutex // защищает УКАЗАТЕЛИ root/canvas/bgSrc (короткие секции)

	// frameMu сериализует кадр (draw+diff) со структурными операциями над
	// канвасом/шрифтами (SetResolution, RegisterFont*, SetTheme и т.п.).
	// e.mu НЕ удерживается на весь кадр — SetRoot/Root и события не
	// блокируются рендером. Порядок захвата: frameMu → e.mu (никогда
	// наоборот); frameMu — корень порядка замков (см. шапку файла).
	frameMu sync.Mutex

	// canvasRef — текущий холст для измерителей (горутины приложения): e.canvas
	// меняют SetScale/SetResolution под e.mu, а измеритель зовут и из кода,
	// который этот замок держит (раскладка корня).
	canvasRef atomic.Pointer[Canvas]
	// scaleBits — текущий HiDPI-масштаб (math.Float64bits) для lock-free
	// чтения из InvalidateRect/toLogical: они вызываются из сеттеров
	// виджетов, в т.ч. когда движок уже держит e.mu (например, root.SetBounds
	// из SetResolution) — брать e.mu там нельзя (RWMutex нереентерабелен).
	scaleBits atomic.Uint64
	// scaleWithin — физический размер холста первичен, логический выводится
	// из масштаба (scalewithin.go). Под e.mu.
	scaleWithin bool
	userDPI     float64 // пользовательский DPI шрифтов (без учёта HiDPI-масштаба)

	// subpixelOwn — подпиксельный текст включила или выключила тема, а не
	// приложение; subpixelPinned — приложение распорядилось само
	// (SetTextSubpixel), и тема режим больше не трогает; subpixelWant — что
	// просил последний применённый профиль. Под frameMu.
	subpixelOwn, subpixelPinned, subpixelWant bool

	// rec — рекордер команд пути Draw (cmdpath.go); cmdPath — включён ли он;
	// replayer и replayDmg — воспроизводитель и буфер повреждения кадра.
	rec       *hgdraw.Recorder
	cmdPath   atomic.Bool
	replayer  hgdraw.Replayer
	replayDmg [1]image.Rectangle
	// tiled — тайловый растр (tileframe.go); tr — его воркеры и кэш; bgGen
	// растёт при смене фона, обоев и формата байтов — растры кэша размытия
	// сняты с прежних.
	tiled atomic.Bool
	tr    *tileRaster
	bgGen atomic.Uint64
	// hashDiff — хэш-дифф тайлов (hashdiff.go); plan — план текущего кадра
	// (переиспользуется, без аллокаций на кадр).
	hashDiff atomic.Bool
	plan     framePlan
	// single — один буфер кадра (ADR-0007, toplayers.go, singlebuf.go холста).
	single atomic.Bool

	// surfaces — буферы SetSurfaces (surfaces.go); свой замок surfaces.mu.
	surfaces surfaceSet

	// videoBuf, overBuf — рабочие буферы поиска видео (video.go). Только
	// горутина кадра.
	videoBuf []widget.VideoSurface
	overBuf  []image.Rectangle

	// moveMu защищает pendingMoves: объявления приходят из горутины, где
	// двигают виджет (обработка ввода), а забирает их горутина кадра.
	moveMu sync.Mutex
	// pendingMoves — объявления, накопленные с прошлого кадра. Список свой у
	// каждого движка: раньше он был один на процесс, и перенос окна в одном
	// движке уносило в кадр другого — тот копировал у себя пиксели,
	// которых там нет.
	pendingMoves []widgetMove
	// moveHandle — дескриптор регистрации приёмника объявлений о переносе
	// (widget.RegisterMoveSinkFor в New, снимается в Stop).
	moveHandle uint64

	// measurerHandle — дескриптор регистрации измерителя текста
	// (widget.RegisterTextMeasurer в New, снимается в Stop).
	measurerHandle uint64
	// measureSet — измерители этого движка для его деревьев (ADR-0008, 5.2):
	// виджет с владельцем меряет текст шрифтами и масштабом своего движка.
	measureSet *widget.MeasureSet
	// svgTextHandle — дескриптор регистрации растеризатора <text> для
	// widget/svg (svg.RegisterTextRasterizer в New, снимается в Stop).
	svgTextHandle uint64

	// offscreen — движок создан NewOffscreen и не занимает служб, общих для
	// процесса: измерителя текста, растеризатора <text> в SVG и границ экрана
	// для всплывающих меню.
	offscreen bool

	// themeMgr — менеджер тем последнего ApplyThemeProfile/SetThemeProfile:
	// из него переходы оверлеев и попапов берут токены анимаций.
	themeMgr atomic.Pointer[theme.Manager]

	// contract — проверка контракта отрисовки (drawcontract.go); свой замок.
	contract drawContract
}

// loopState — цикл кадров: горутина, темп и доставка, рендер по запросу и
// повреждение, очередь Post, таймеры, статистика, политика сбоев, профиль
// движения, запись кадров. Будущий core/loop.
type loopState struct {
	fps    int          // целевой FPS, 1–120
	maxFPS atomic.Int32 // потолок частоты кадров поверх fps; 0 — не задан (maxfps.go)

	started atomic.Bool // Start вызывался (Stop без Start не ждёт цикл)
	stopped atomic.Bool // Stop уже выполнен (идемпотентность)
	quit    chan struct{}
	done    chan struct{}

	// pacing, frameWanted, wake, sink — внешний темп и сток кадров
	// (pacing.go). При PacingExternal кадр готовится только по RequestFrame.
	pacing      atomic.Uint32
	frameWanted atomic.Bool
	wake        chan struct{}
	loopGID     atomic.Uint64 // горутина цикла кадров; 0 — цикл не идёт (см. Flush)
	sinkMu      sync.RWMutex
	sink        FrameSink

	frames   chan output.Frame
	frameSeq atomic.Uint64
	// fullNext — следующий кадр полный (холст пересоздан); lostFrame — кадр
	// выброшен из переполненного канала, потребитель отстал (fullframe.go).
	fullNext  atomic.Bool
	lostFrame atomic.Bool
	// recycleFrames — данные тайлов кадров берутся из пула и возвращаются по
	// Frame.Release (SetFrameRecycling, fullframe.go).
	recycleFrames atomic.Bool
	// noTileData — кадр без байтов тайлов (SetFrameTileData).
	noTileData atomic.Bool

	// ── Рендер по запросу (см. invalidate.go) ───────────────────────────────
	onDemand atomic.Bool
	invGen   atomic.Uint64

	damageMu  sync.Mutex
	damage    []image.Rectangle // области InvalidateRect с прошлого кадра
	damageAll bool              // Invalidate() — полный diff
	// force — ничейное повреждение кадра (cmdpath.go): логические области из
	// InvalidateRect, widget.InvalidateRect, фокуса — виджет пути команд,
	// чей охват их задевает, перезаписывается. Под damageMu.
	force []image.Rectangle
	// forceFrame — ничейное повреждение, забранное кадром (буфер
	// переиспользуется: takeForce). Только горутина кадра.
	forceFrame []image.Rectangle
	// notifierHandle — дескриптор регистрации этого движка в широковещательном
	// реестре нотификаторов widget-пакета (RegisterUINotifier в New). Снимается
	// в Stop (UnregisterUINotifier), чтобы короткоживущие движки диалогов/
	// попапов не накапливались в реестре.
	notifierHandle uint64

	// postMu защищает posted: ставят в очередь из любых горутин, разбирает
	// горутина кадра (см. post.go).
	postMu sync.Mutex
	// posted — вызовы, ожидающие выполнения в горутине кадра.
	posted []func()

	// timersMu защищает timers — таймеры After/Every, живые у этого движка. Stop
	// гасит их все (см. timer.go).
	timersMu sync.Mutex
	timers   map[*Timer]struct{}

	// stats — счётчики для Engine.Stats (stats.go).
	stats engineStats
	// failure — политика сбоев (failure.go); свой замок.
	failure failureState
	// motion — профиль движения (MotionProfile, motionprofile.go);
	// motionCounted — движок учтён в motionEngines.
	motion        atomic.Uint32
	motionCounted atomic.Bool

	saveDir   string        // если не пусто — сохранять PNG в эту директорию
	saveCh    chan saveJob  // канал для асинхронного сохранения
	saveDone  chan struct{} // закрывается, когда saveWorker завершил запись всех PNG
	saveLimit atomic.Int64  // предел числа сохраняемых кадров (0 — без предела)
	saveCount atomic.Int64
	saveOnce  sync.Once // лог о достижении предела — один раз
	snapPool  sync.Pool // пул снапшотов front-буфера для SaveFrames
}

// overlayState — слои поверх дерева: модальные окна в холсте и их хост,
// хостовые попапы с ареной и переходами, отложенные слои кадра, переходы
// оверлеев, всплывающая подсказка. Будущий core/overlay.
type overlayState struct {
	modals []widget.ModalWidget // стек модальных виджетов (последний = верхний)
	modMu  sync.Mutex

	// modalHost — опциональный хост нативных модалок (window.dialogHost на
	// Win32). Если установлен и принимает модалку, она живёт в собственном
	// нативном окне ОС, а не в холсте движка. onModalClosed — колбэк,
	// вызываемый в конце CloseModal (см. modalhost.go). hostMu защищает оба.
	modalHost     ModalHost
	onModalClosed func(widget.ModalWidget)
	// modalClosedSubs — подписчики AddOnModalClosed. Слот onModalClosed занят
	// хостом нативных окон, и приложению его не хватало: второй подписчик
	// молча вытеснял первого (GG-82).
	modalClosedSubs []modalClosedSub
	modalSubNextID  int
	hostMu          sync.Mutex

	// popupSink — опциональный хост popup-оверлеев (window.popupHost). Если
	// задан, открытые оверлеи (dropdown/меню) выносятся в собственные нативные
	// окна ОС, а не рисуются в холст. lastPopupSig — сигнатура последнего
	// набора попапов, отданного sink (для вызова sink только при изменениях).
	popupSink    func([]PopupFrame)
	lastPopupSig []popupSig
	popupCache   map[uintptr]*popupEntry // кэш картинки на оверлей
	// popupArena — общий холст-заготовка хостовых попапов (ADR-0009); только
	// горутина кадра, под frameMu.
	popupArena popupArena
	popupGen   uint64 // поколение для отсева исчезнувших попапов
	popupMu    sync.Mutex

	// Рабочие буферы renderPopups (только рендер-горутина, под frameMu).
	popupItems []popupItem
	popupSigBu []popupSig
	// Переходы хостовых попапов (popuptrans.go): popupFade — хост их
	// понимает (SetPopupFade); popupTicked — тик просит кадр под шаг;
	// popupClosingBu — исчезающие попапы кадра (переиспользуемый буфер).
	popupFade      atomic.Bool
	popupTicked    atomic.Bool
	popupClosingBu []uintptr

	// post, postRect, postMode, postTooltip — слои, отложенные до конца растра
	// (переходы оверлеев, toplayers.go), их место и признак, что обход уже
	// откладывает. Только горутина кадра.
	post        []postItem
	postRect    image.Rectangle
	postMode    bool
	postTooltip tooltipPlan

	// ovt — переходы оверлеев (overlaytrans.go); у каждого перехода свой замок.
	ovt overlayTransitions

	// ── Tooltip ─────────────────────────────────────────────────────────────
	ttMu       sync.Mutex
	ttEnabled  bool          // показывать всплывающие подсказки
	ttDelay    time.Duration // задержка появления после остановки курсора
	ttMouseX   int
	ttMouseY   int
	ttLastMove time.Time
	ttHasMouse bool            // курсор хотя бы раз входил на холст
	ttShownAt  image.Rectangle // область показанной подсказки (пустая — не показана)
}
