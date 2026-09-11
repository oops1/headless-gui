// socialpreview — картинка для превью репозитория (social_preview.png).
//
//	go run ./cmd/socialpreview                  — записать social_preview.png
//	go run ./cmd/socialpreview -out кадр.png    — в другой файл
//
// Всё, кроме фона, рисует сам движок и без окна: настоящий widget.Window с
// контролом сравнения DiffView, стеклянная панель с размытием подложки
// (BlurBehind), плашки возможностей и строка на нескольких письменностях через
// шейпинг. Фон — background.jpg, сгенерирован FLUX.2 [klein] (seed 3172) и
// вкомпилирован, поэтому картинка пересобирается без модели.
//
// Запускать из корня репозитория: заголовок набран Inter Bold из assets/fonts.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

//go:embed background.jpg
var backgroundJPEG []byte

const W, H = 1280, 640 // размер превью, который рекомендует GitHub

const (
	leftText = `func New(s string) *Btn {
	b := &Btn{Text: s}
	b.Pad = 8
	return b
}

func (b *Btn) Draw(c Ctx) {
	c.Fill(b.R, b.BG)
	c.Text(b.Text, b.FG)
}

// drawn by headless-gui
`
	rightText = `func New(s string) *Btn {
	b := &Btn{Text: s}
	b.Pad = 10
	b.Radius = 6
	return b
}

func (b *Btn) Draw(c Ctx) {
	c.Round(b.R, b.BG)
	c.Text(b.Text, b.FG)
	c.Focus(b)
}

// drawn by headless-gui
`
)

// chip — плашка возможности: подпись и её цвет.
type chip struct {
	text string
	col  color.RGBA
}

var (
	blue   = color.RGBA{0x7c, 0xb4, 0xff, 0xff}
	violet = color.RGBA{0xc3, 0x9b, 0xff, 0xff}
	green  = color.RGBA{0x7e, 0xe0, 0xa8, 0xff}
	orange = color.RGBA{0xff, 0xb8, 0x6b, 0xff}
	teal   = color.RGBA{0x6f, 0xe3, 0xe0, 0xff}
	pink   = color.RGBA{0xff, 0x9c, 0xc8, 0xff}

	chips = []chip{
		{"WPF / XAML", blue},
		{"Text shaping", violet},
		{"HiDPI", green},
		{"DiffView", orange},
		{"Docking", teal},
		{"Themes as data", pink},
		{"AVX2 kernels", green},
		{"Accessibility", blue},
		{"Wayland + X11", violet},
	}
)

// scene — корень кадра: фон, тень окна, стеклянная панель с текстом. Окно с
// контролом сравнения — ребёнок, рисуется поверх.
type scene struct {
	widget.Base
	bg    image.Image
	win   *widget.Window
	glass image.Rectangle
	title string // шрифт заголовка
}

func (s *scene) Draw(ctx widget.DrawContext) {
	ctx.DrawImageScaled(s.bg, 0, 0, W, H)

	// Тень под окном — окно ОС отбрасывало бы её само.
	if sd, ok := ctx.(widget.ShadowDrawer); ok {
		sd.DrawSoftShadow(s.win.Bounds(), 8, 24, color.RGBA{0, 0, 0, 0xb0})
	}
	s.drawGlass(ctx)
	s.Base.DrawChildren(ctx)
}

// drawGlass — панель матового стекла: подложка размыта и притемнена, поверх —
// заголовок, плашки, строка разных письменностей и подпись платформ.
func (s *scene) drawGlass(ctx widget.DrawContext) {
	g := s.glass
	const corner = 20
	// SetRoundClip подменяет и прямоугольный клип, а ClearRoundClip снимает
	// только скругление — прежний клип возвращаем сами, иначе окно, которое
	// рисуется следом, обрезалось бы границами панели.
	prev := ctx.Clip()
	if rc, ok := ctx.(widget.RoundClipper); ok {
		rc.SetRoundClip(g, corner)
	}
	tint := color.RGBA{0x0a, 0x0c, 0x1a, 0x78}
	if bd, ok := ctx.(widget.BackdropDrawer); ok {
		bd.BlurBehind(g, 32, tint)
	} else {
		ctx.FillRectAlpha(g.Min.X, g.Min.Y, g.Dx(), g.Dy(), tint)
	}
	if rc, ok := ctx.(widget.RoundClipper); ok {
		rc.ClearRoundClip()
	}
	ctx.SetClip(prev)
	ctx.DrawRoundBorder(g.Min.X, g.Min.Y, g.Dx(), g.Dy(), corner, color.RGBA{0x2a, 0x2e, 0x44, 0x60})

	x := g.Min.X + 36
	y := g.Min.Y + 30

	ctx.DrawTextFont("headless-gui", x-2, y, 46, s.title, color.RGBA{0xf2, 0xf4, 0xfb, 0xff})
	y += 76
	// Черта-акцент: переход от голубого к фиолетовому.
	for i := 0; i < 132; i++ {
		ctx.FillRect(x+i, y, 1, 4, lerp(blue, violet, float64(i)/131))
	}
	y += 26

	muted := color.RGBA{0xa8, 0xae, 0xc4, 0xff}
	ctx.DrawTextSize("Pure-Go GUI engine — zero CGO, rendered off-screen", x, y, 15, muted)
	y += 46

	// Плашки — строками, с переносом по ширине панели.
	const chipH, chipPad, gap, chipPt = 32, 16, 12, 12.0
	cx, maxX := x, g.Max.X-36
	for _, c := range chips {
		w := ctx.MeasureText(c.text, chipPt) + 2*chipPad
		if cx+w > maxX {
			cx = x
			y += chipH + gap
		}
		fill := c.col
		fill.A = 0x1c
		ctx.FillRoundRect(cx, y, w, chipH, chipH/2, premul(fill))
		border := c.col
		border.A = 0x70
		ctx.DrawRoundBorder(cx, y, w, chipH, chipH/2, premul(border))
		ctx.DrawTextSize(c.text, cx+chipPad, y+8, chipPt, c.col)
		cx += w + gap
	}
	y += chipH + 38

	ctx.DrawTextSize("Привет · Hello · שלום · مرحبا بالعالم · नमस्ते · สวัสดี", x, y, 16, color.RGBA{0xe6, 0xe8, 0xf0, 0xff})
	y += 38
	ctx.FillRectAlpha(x, y, g.Dx()-72, 1, color.RGBA{0x40, 0x44, 0x5c, 0x80})
	y += 22
	ctx.DrawTextSize("Windows · Linux (X11 / Wayland) · macOS · WebSocket streaming", x, y, 12, muted)
}

func main() {
	out := flag.String("out", "social_preview.png", "куда записать картинку")
	fonts := flag.String("fonts", "assets/fonts", "каталог со шрифтом Inter Bold")
	flag.Parse()

	bg, err := jpeg.Decode(bytes.NewReader(backgroundJPEG))
	if err != nil {
		log.Fatal(err)
	}
	rgba := image.NewRGBA(bg.Bounds())
	draw.Draw(rgba, rgba.Bounds(), bg, image.Point{}, draw.Src)

	eng := engine.New(W, H, 30)
	title := ""
	if err := eng.RegisterFontFile("InterBold", filepath.Join(*fonts, "Inter-Bold.ttf")); err == nil {
		title = "InterBold"
	} else {
		log.Printf("Inter Bold не найден (%v) — заголовок встроенным жирным", err)
		title = widget.BuiltinFontBold
	}
	eng.SetTheme(widget.ThemeByName("Win11 Dark"))

	dv := widget.NewDiffView("", "")
	dv.SetFont("", "", 9)
	dv.SetText(widget.DiffLeft, "main", "button.go", leftText)
	dv.SetText(widget.DiffRight, "feature/round", "button.go", rightText)
	dv.SetSyntaxHighlight(true)

	win := widget.NewWindow("button.go — DiffView", 628, 456)
	win.TitleStyle = widget.WindowTitleWin
	win.ShowLocaleIndicator = false
	win.MainWindow = false
	win.AddChild(dv)

	s := &scene{
		bg:    rgba,
		win:   win,
		glass: image.Rect(40, 92, 580, 548),
		title: title,
	}
	s.AddChild(win)
	s.SetBounds(image.Rect(0, 0, W, H))
	win.ApplyTheme(widget.CurrentTheme())
	win.SetBounds(image.Rect(612, 92, W-40, 548))
	eng.SetRoot(s)
	win.SetFrameColor(color.RGBA{0x4c, 0x8d, 0xff, 0xff}) // рамка акцентом

	img := eng.RenderOnce()
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	fmt.Println("записано:", *out)
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

// premul — цвет с альфой в premultiplied-виде, как ждёт холст движка.
func premul(c color.RGBA) color.RGBA {
	p := func(v uint8) uint8 { return uint8(uint16(v) * uint16(c.A) / 255) }
	return color.RGBA{p(c.R), p(c.G), p(c.B), c.A}
}
