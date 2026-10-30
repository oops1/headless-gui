package base

// bgcolor.go — собственный фон контейнера как возможность (ADR-0012, 8.5):
// заголовок окна с вкладками сращивает корешок с фоном содержимого, не зная
// его типа.

import "image/color"

// BackgroundColorer — виджет со своим фоном: BackgroundColor отдаёт цвет и
// задан ли он (прозрачный — нет, виден фон родителя).
type BackgroundColorer interface {
	BackgroundColor() (color.RGBA, bool)
}

// BackgroundColor — фон панели (BackgroundColorer).
func (p *Panel) BackgroundColor() (color.RGBA, bool) { return p.Background, p.Background.A > 0 }

// BackgroundColor — фон холста (BackgroundColorer).
func (c *Canvas) BackgroundColor() (color.RGBA, bool) { return c.Background, c.Background.A > 0 }

// BackgroundColor — фон сетки (BackgroundColorer).
func (g *Grid) BackgroundColor() (color.RGBA, bool) { return g.Background, g.Background.A > 0 }

var (
	_ BackgroundColorer = (*Panel)(nil)
	_ BackgroundColorer = (*Canvas)(nil)
	_ BackgroundColorer = (*Grid)(nil)
)
