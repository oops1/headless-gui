package widget

import (
	"image/color"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// Тесты разбора HTML в абзацы (вставка из буфера обмена). Дамп — тот же, что в
// richtext_doc_test.go: оформление, текст в кавычках, абзацы через «¶».

func rdParse(src string) string { return rdDumpParas(RichParagraphsFromHTML(src)) }

func TestRichHTML_Table(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// Абзацы и переводы строк.
		{"абзац", `<p>Hello</p>`, `-"Hello"`},
		{"два абзаца", `<p>a</p><p>b</p>`, `-"a" ¶ -"b"`},
		{"div", `<div>a</div><div>b</div>`, `-"a" ¶ -"b"`},
		{"вложенные div", `<div>a<div>b</div>c</div>`, `-"a" ¶ -"b" ¶ -"c"`},
		{"голый текст", `just text`, `-"just text"`},
		{"p без закрытия", `<p>a<p>b<p>c`, `-"a" ¶ -"b" ¶ -"c"`},
		{"br внутри", `<p>a<br>b</p>`, `-"a\nb"`},
		{"br в конце блока не создаёт строку", `<p>a<br></p>`, `-"a"`},
		{"два br в конце", `<p>a<br><br></p>`, `-"a\n"`},
		{"только br — пустая строка", `<p><br></p>`, `()`},
		{"br без абзаца", `a<br>b`, `-"a\nb"`},
		{"пробел после br отброшен", `<p>a<br> b</p>`, `-"a\nb"`},
		{"пробел перед br отброшен", `<p>a <br>b</p>`, `-"a\nb"`},
		{"br/ и BR", `<p>a<br/>b<BR>c<br />d</p>`, `-"a\nb\nc\nd"`},
		{"пустой p не даёт абзаца", `<p>a</p><p></p><p>b</p>`, `-"a" ¶ -"b"`},
		{"пустая строка Gmail", `<div>a</div><div><br></div><div>b</div>`, `-"a" ¶ () ¶ -"b"`},
		{"пустая строка Word", `<p>a</p><p><o:p>&nbsp;</o:p></p><p>b</p>`, `-"a" ¶ -" " ¶ -"b"`},

		// Заголовки.
		{"h1", `<h1>Title</h1>`, `b+sz24"Title"`},
		{"h3 с вложенным курсивом", `<h3>A <i>b</i></h3>`, `b+sz14"A " bi+sz14"b"`},
		{"h6", `<h6>x</h6>`, `b+sz8"x"`},
		{"заголовок и абзац", `<h2>H</h2><p>t</p>`, `b+sz18"H" ¶ -"t"`},
		{"заголовок со своим кеглем", `<h1 style="font-size:20pt">H</h1>`, `b+sz20"H"`},

		// Строчные теги.
		{"b", `<p>a<b>b</b>c</p>`, `-"a" b"b" -"c"`},
		{"strong", `<strong>x</strong>`, `b"x"`},
		{"i", `<i>x</i>`, `i"x"`},
		{"em", `<em>x</em>`, `i"x"`},
		{"u", `<u>x</u>`, `u"x"`},
		{"s", `<s>x</s>`, `s"x"`},
		{"strike", `<strike>x</strike>`, `s"x"`},
		{"del", `<del>x</del>`, `s"x"`},
		{"ins", `<ins>x</ins>`, `u"x"`},
		{"вложенность b i", `<b>a<i>b</i>c</b>`, `b"a" bi"b" b"c"`},
		{"глубокая вложенность", `<b><i><u><s>x</s></u></i></b>`, `bi+u+s"x"`},
		{"разное оформление подряд", `<b>a</b><i>b</i><u>c</u>`, `b"a" i"b" u"c"`},
		{"одинаковое оформление сливается", `<b>a</b><b>b</b><strong>c</strong>`, `b"abc"`},
		{"code", `<p>x <code>y</code></p>`, `-"x " m"y"`},
		{"mark", `<mark>x</mark>`, `bgffff00"x"`},
		{"неизвестный тег прозрачен", `<p>a<blink>b</blink><o:p>c</o:p></p>`, `-"abc"`},

		// Ссылки.
		{"ссылка", `<a href="https://example.com/x?a=1&amp;b=2">go</a>`, `link=https://example.com/x?a=1&b=2"go"`},
		{"ссылка без href", `<a name="top">x</a>`, `-"x"`},
		{"ссылка с b", `<a href="http://a"><b>x</b></a>`, `b+link=http://a"x"`},
		{"javascript:", `<a href="javascript:alert(1)">x</a>`, `-"x"`},
		{"JavaScript: с пробелами", `<a href=" Java	Script:alert(1)">x</a>`, `-"x"`},
		{"vbscript:", `<a href="vbscript:x">x</a>`, `-"x"`},
		{"data:", `<a href="data:text/html,<b>">x</a>`, `-"x"`},
		{"mailto", `<a href="mailto:a@b.c">m</a>`, `link=mailto:a@b.c"m"`},
		{"якорь", `<a href="#sec">s</a>`, `link=#sec"s"`},
		{"href без кавычек", `<a href=http://a/b>x</a>`, `link=http://a/b"x"`},
		{"href в одинарных", `<a href='http://a'>x</a>`, `link=http://a"x"`},
		{"пустой href", `<a href="">x</a>`, `-"x"`},

		// style.
		{"цвет #rrggbb", `<span style="color:#ff0000">x</span>`, `cff0000"x"`},
		{"цвет #rgb", `<span style="color:#0f0">x</span>`, `c00ff00"x"`},
		{"цвет rgb()", `<span style="color: rgb(0, 0, 255)">x</span>`, `c0000ff"x"`},
		{"цвет имя", `<span style="color:Red">x</span>`, `cff0000"x"`},
		{"цвет неизвестный игнорируется", `<span style="color:nonsense">x</span>`, `-"x"`},
		{"transparent не красит", `<span style="color:transparent">x</span>`, `-"x"`},
		{"фон", `<span style="background-color:#00ff00">x</span>`, `bg00ff00"x"`},
		{"фон shorthand", `<span style="background:yellow url(a.png) no-repeat">x</span>`, `bgffff00"x"`},
		{"кегль pt", `<span style="font-size:14pt">x</span>`, `sz14"x"`},
		{"кегль px", `<span style="font-size:16px">x</span>`, `sz12"x"`},
		{"кегль слово", `<span style="font-size:large">x</span>`, `sz13.5"x"`},
		{"кегль em от родителя", `<span style="font-size:20pt">a<span style="font-size:0.5em">b</span></span>`, `sz20"a" sz10"b"`},
		{"кегль мусор", `<span style="font-size:bigger">x</span>`, `-"x"`},
		{"кегль ноль", `<span style="font-size:0">x</span>`, `-"x"`},
		{"вес bold", `<span style="font-weight:bold">x</span>`, `b"x"`},
		{"вес 700", `<span style="font-weight: 700">x</span>`, `b"x"`},
		{"вес 400 снимает", `<b>a<span style="font-weight:400">b</span></b>`, `b"a" -"b"`},
		{"курсив", `<span style="font-style:italic">x</span>`, `i"x"`},
		{"oblique", `<span style="font-style:oblique">x</span>`, `i"x"`},
		{"подчёркивание", `<span style="text-decoration:underline">x</span>`, `u"x"`},
		{"оба", `<span style="text-decoration: underline line-through">x</span>`, `u+s"x"`},
		{"line", `<span style="text-decoration-line:line-through">x</span>`, `s"x"`},
		{"none не снимает", `<u>a<span style="text-decoration:none">b</span></u>`, `u"ab"`},
		{"important", `<span style="color:#f00 !important">x</span>`, `cff0000"x"`},
		{"заглавные свойства", `<span STYLE="COLOR:#F00;Font-Weight:BOLD">x</span>`, `b+cff0000"x"`},
		{"точка с запятой в кавычках", `<span style="font-family:'a;b';color:#f00">x</span>`, `cff0000"x"`},
		{"мусорные объявления", `<span style=";;color;:;font-size:;color:#00f;;">x</span>`, `c0000ff"x"`},
		{"наследование вниз", `<span style="color:#f00"><b>a</b><span style="color:#00f">b</span></span>`, `b+cff0000"a" c0000ff"b"`},
		{"font color size", `<font color="#00ff00" size="5">x</font>`, `sz18+c00ff00"x"`},
		{"font size относительный", `<font size="+1">x</font>`, `sz13.5"x"`},
		{"font face моно", `<font face="Courier New">x</font>`, `m"x"`},

		// Шрифты.
		{"семейство не переносится", `<span style="font-family:Calibri, Arial">x</span>`, `-"x"`},
		{"моноширинный", `<span style="font-family:'Courier New',monospace">x</span>`, `m"x"`},
		{"consolas", `<span style="font-family:Consolas">x</span>`, `m"x"`},
		{"жирный не теряется при семействе", `<b style="font-family:Arial">x</b>`, `b"x"`},
		{"pre", `<pre>a  b
c</pre>`, `m"a  b\nc"`},

		// Абзацы: выравнивание и отступы.
		{"text-align center", `<p style="text-align:center">x</p>`, `[c] -"x"`},
		{"align right", `<p align="right">x</p>`, `[r] -"x"`},
		{"align justify", `<p align="justify">x</p>`, `-"x"`},
		{"center тег", `<center>x</center>`, `[c] -"x"`},
		{"выравнивание наследуется от div", `<div style="text-align:right"><p>a</p><p style="text-align:left">b</p></div>`, `[r] -"a" ¶ -"b"`},
		{"поля px", `<p style="margin-top:3px;margin-bottom:4px;margin-left:20px">x</p>`, `[ind=20 sb=3 sa=4] -"x"`},
		{"поля pt", `<p style="margin-left:15pt">x</p>`, `[ind=20] -"x"`},
		{"margin shorthand 4", `<p style="margin:1px 2px 3px 4px">x</p>`, `[ind=4 sb=1 sa=3] -"x"`},
		{"margin shorthand 2", `<p style="margin:5px 9px">x</p>`, `[ind=9 sb=5 sa=5] -"x"`},
		{"margin auto игнорируется", `<p style="margin:0 auto">x</p>`, `-"x"`},
		{"отрицательные поля", `<p style="margin-left:-9px">x</p>`, `-"x"`},
		{"padding-left", `<p style="padding-left:10px">x</p>`, `[ind=10] -"x"`},
		{"отступы вложенных div складываются", `<div style="margin-left:10px"><p style="margin-left:5px">x</p></div>`, `[ind=15] -"x"`},

		// Списки.
		{"ul", `<ul><li>a</li><li>b</li></ul>`, `[ind=24] -"• a" ¶ [ind=24] -"• b"`},
		{"ol", `<ol><li>a</li></ol>`, `[ind=24] -"• a"`},
		{"вложенный список", `<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>`, `[ind=24] -"• a" ¶ [ind=48] -"• b" ¶ [ind=24] -"• c"`},
		{"li без закрытия", `<ul><li>a<li>b</ul>`, `[ind=24] -"• a" ¶ [ind=24] -"• b"`},
		{"li с форматированием", `<ul><li><b>x</b> y</li></ul>`, `[ind=24] -"• " b"x" -" y"`},
		{"li с p", `<ul><li><p>x</p></li></ul>`, `[ind=24] -"• x"`},
		{"пустой li без маркера", `<ul><li></li></ul><p>x</p>`, `-"x"`},
		{"маркер без подчёркивания ссылки", `<ul><li><a href="http://a">x</a></li></ul>`, `[ind=24] -"• " link=http://a"x"`},
		{"Word-список", `<p style="margin-left:36pt;text-indent:-18pt;mso-list:l0 level1 lfo1"><![if !supportLists]><span style="mso-list:Ignore">·<span>&nbsp;&nbsp;</span></span><![endif]>пункт</p>`, `[ind=48] -"• пункт"`},
		{"blockquote", `<blockquote>q</blockquote>`, `[ind=32] -"q"`},

		// Таблицы.
		{"таблица", `<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`, `-"a b" ¶ -"c d"`},

		// Сущности.
		{"базовые сущности", `<p>a &amp; b &lt;c&gt; &quot;d&quot; &#39;e&#39;</p>`, `-"a & b <c> \"d\" 'e'"`},
		{"числовые", `<p>&#1087;&#x440;&#x1F600;</p>`, `-"пр😀"`},
		{"nbsp не схлопывается", `<p>a&nbsp;&nbsp;&nbsp;b</p>`, `-"a   b"`},
		{"nbsp в начале сохраняется", `<p>&nbsp;&nbsp;a</p>`, `-"  a"`},
		{"nbsp в конце сохраняется", `<p>a&nbsp;</p>`, `-"a "`},
		{"неизвестная сущность", `<p>a &nosuch; b</p>`, `-"a &nosuch; b"`},
		{"сущность без точки с запятой в конце", `<p>x &amp y</p>`, `-"x & y"`},
		{"сущности в атрибуте", `<span style="color:&#35;f00">x</span>`, `cff0000"x"`},
		{"mdash и копирайт", `<p>a&mdash;b &copy;</p>`, `-"a—b ©"`},

		// Пробелы.
		{"схлопывание", "<p>  a   b \n c  </p>", `-"a b c"`},
		{"пробелы между тегами", `<p><b>a</b> <i>b</i></p>`, `b"a" -" " i"b"`},
		{"пробел внутри рана до тега", `<p><b>a </b><i>b</i></p>`, `b"a " i"b"`},
		{"двойной пробел через границу", `<p><b>a </b> <i>b</i></p>`, `b"a " i"b"`},
		{"пробелы между блоками", "<p>a</p>\n  \n<p>b</p>\n", `-"a" ¶ -"b"`},
		{"табуляция", "<p>a\t\tb</p>", `-"a b"`},
		{"только пробелы", `<p>   </p>`, ``},
		{"пробелы вокруг абзаца", `<p> a </p>`, `-"a"`},
		{"перевод строки в тексте", "<p>a\nb\r\nc</p>", `-"a b c"`},

		// Выбрасываемое.
		{"комментарий", `<p>a<!-- скрыто -->b</p>`, `-"ab"`},
		{"маркеры фрагмента", `<html><body><!--StartFragment--><p>x</p><!--EndFragment--></body></html>`, `-"x"`},
		{"style", `<style>p{color:red}</style><p>x</p>`, `-"x"`},
		{"script", `<script>var a = "<p>no</p>";</script><p>x</p>`, `-"x"`},
		{"SCRIPT регистр", `<SCRIPT>if (a<b) {}</SCRIPT>x`, `-"x"`},
		{"head и title", `<html><head><title>T</title><meta charset="utf-8"></head><body>x</body></html>`, `-"x"`},
		{"head без закрытия", `<head><title>T</title><body>x`, `-"x"`},
		{"doctype", `<!DOCTYPE html><p>x</p>`, `-"x"`},
		{"условный комментарий Word", `<!--[if gte mso 9]><xml><o:OfficeDocumentSettings></o:OfficeDocumentSettings></xml><![endif]--><p>x</p>`, `-"x"`},
		{"xml-инструкция", `<?xml version="1.0"?><p>x</p>`, `-"x"`},
		{"display:none", `<p>a<span style="display:none">секрет</span>b</p>`, `-"ab"`},
		{"hidden", `<p>a<span hidden>секрет</span>b</p>`, `-"ab"`},
		{"картинка", `<p>a<img src="x.png" alt="pic">b</p>`, `-"ab"`},
		{"svg", `<p>a<svg><text>T</text></svg>b</p>`, `-"ab"`},
		{"iframe", `<p>a<iframe src="x">fallback</iframe>b</p>`, `-"ab"`},

		// Мусор.
		{"незакрытые теги", `<p><b>bold <i>both`, `b"bold " bi"both"`},
		{"лишний закрывающий", `<p>a</b></i></div>b</p>`, `-"ab"`},
		{"перекрёстная вложенность", `<b>a<i>b</b>c</i>`, `b"a" bi"b" -"c"`},
		{"голый <", `<p>1 < 2 and 3 > 2</p>`, `-"1 < 2 and 3 > 2"`},
		{"< в конце", `abc<`, `-"abc<"`},
		{"тег не закрыт до конца", `<p>abc<b class="x`, `-"abc"`},
		{"значение атрибута не закрыто", `<p>abc<a href="x>def</p>`, `-"abc"`},
		{"комментарий не закрыт", `<p>abc<!-- не закрыт <b>x</b>`, `-"abc"`},
		{"атрибуты без значений", `<p hidden2 data-x=1 class>x</p>`, `-"x"`},
		{"= в атрибутах", `<p ==="" a=b=c>x</p>`, `-"x"`},
		{"пустая строка", ``, ``},
		{"только теги", `<p></p><div><b></b></div>`, ``},
		{"только пробелы и комментарий", ` <!-- x --> `, ``},
		{"управляющие символы", "<p>a\x00b\x07c​</p>", `-"abc\u200b"`},
		{"p/ самозакрытый", `<p/>x`, `-"x"`},
		{"span/ самозакрытый", `<span/>x`, `-"x"`},
		{"кириллица и эмодзи", `<p><b>Привет</b>, мир 😀 日本語</p>`, `b"Привет" -", мир 😀 日本語"`},
		{"верхний регистр тегов", `<P ALIGN=CENTER><B>X</B></P>`, `[c] b"X"`},
	}
	for _, c := range cases {
		got := rdParse(c.in)
		if got != c.want {
			t.Errorf("%s\n  вход     %q\n  получили %s\n  ждали    %s", c.name, c.in, got, c.want)
		}
		// Результат любого разбора — в каноническом виде документа.
		for _, p := range RichParagraphsFromHTML(c.in) {
			if richParaLen(p) > 0 {
				for j, r := range p.Runs {
					if r.Text == "" || j > 0 && richSameStyle(p.Runs[j-1], r) {
						t.Errorf("%s: раны не приведены к инвариантам: %s", c.name, rdDumpParas([]RichParagraph{p}))
					}
				}
			}
		}
	}
}

func TestRichHTML_NoResultIsNil(t *testing.T) {
	for _, s := range []string{"", "  ", "<p></p>", "<!-- x -->", "<style>a{}</style>"} {
		if got := RichParagraphsFromHTML(s); got != nil {
			t.Errorf("%q: %v, ждали nil", s, got)
		}
	}
}

func TestRichHTML_Colors(t *testing.T) {
	cases := []struct {
		in   string
		want color.RGBA
		ok   bool
	}{
		{"#fff", color.RGBA{255, 255, 255, 255}, true},
		{"#FF8000", color.RGBA{255, 128, 0, 255}, true},
		{"rgb(10,20,30)", color.RGBA{10, 20, 30, 255}, true},
		{"rgb(10 20 30)", color.RGBA{10, 20, 30, 255}, true},
		{"rgb(100%,0%,50%)", color.RGBA{255, 0, 128, 255}, true},
		{"rgb(300,-5,0)", color.RGBA{255, 0, 0, 255}, true},
		// Альфа: движок хранит цвет с предумноженной альфой.
		{"rgba(255,0,0,0.5)", color.RGBA{128, 0, 0, 128}, true},
		{"rgba(255,255,255,1)", color.RGBA{255, 255, 255, 255}, true},
		{"rgba(10,20,30,0)", color.RGBA{}, true},
		{"#ff000080", color.RGBA{128, 0, 0, 128}, true},
		{"transparent", color.RGBA{}, true},
		{"navy", color.RGBA{0, 0, 128, 255}, true},
		{"#ggg", color.RGBA{}, false},
		{"#12345", color.RGBA{}, false},
		{"rgb(1,2)", color.RGBA{}, false},
		{"rgb(a,b,c)", color.RGBA{}, false},
		{"", color.RGBA{}, false},
		{"inherit", color.RGBA{}, false},
	}
	for _, c := range cases {
		got, ok := rhParseColor(c.in)
		if ok != c.ok || ok && got != c.want {
			t.Errorf("rhParseColor(%q) = %v, %v; ждали %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestRichHTML_ParagraphFormatsOfHeadings(t *testing.T) {
	ps := RichParagraphsFromHTML(`<h1 style="text-align:center;margin-bottom:6px">Заголовок</h1>`)
	if len(ps) != 1 || ps[0].Align != TextAlignCenter || ps[0].SpaceAfter != 6 {
		t.Fatalf("%+v", ps)
	}
	r := ps[0].Runs[0]
	if r.Font != BuiltinFontBold || r.Size != 24 {
		t.Errorf("заголовок: %+v", r)
	}
}

func TestRichHTML_CFHTMLFragmentRoundTrip(t *testing.T) {
	// Фрагмент из буфера Windows: рамка документа и маркеры.
	frag := BuildCFHTML(`<p>Привет <b>мир</b></p>`)
	html, ok := ParseCFHTML(frag)
	if !ok {
		t.Fatal("фрагмент не разобран")
	}
	if got := rdParse(html); got != `-"Привет " b"мир"` {
		t.Errorf("%s", got)
	}
}

func TestRichHTML_WordSample(t *testing.T) {
	// Типичный фрагмент Word: классы, mso-свойства, o:p, неразрывные пробелы.
	src := `<html xmlns:o="urn:schemas-microsoft-com:office:office">
<head><meta name=Generator content="Microsoft Word 15"><style><!--
 p.MsoNormal {margin:0cm; font-size:11.0pt; font-family:"Calibri",sans-serif;}
--></style></head>
<body lang=RU>
<!--StartFragment-->
<p class=MsoNormal align=center style='text-align:center'><b><span style='font-size:14.0pt;color:#C00000'>Заголовок</span></b><o:p></o:p></p>
<p class=MsoNormal><span style='font-family:"Calibri",sans-serif'>Обычный </span><i><span style='font-family:"Calibri",sans-serif'>курсив</span></i><span style='font-family:"Calibri",sans-serif'> и </span><a href="https://example.com"><span style='font-family:"Calibri",sans-serif'>ссылка</span></a><o:p></o:p></p>
<p class=MsoNormal><o:p>&nbsp;</o:p></p>
<!--EndFragment-->
</body></html>`
	want := `[c] b+sz14+cc00000"Заголовок" ¶ -"Обычный " i"курсив" -" и " link=https://example.com"ссылка" ¶ -" "`
	if got := rdParse(src); got != want {
		t.Errorf("\n  получили %s\n  ждали    %s", got, want)
	}
}

func TestRichHTML_ChromeSample(t *testing.T) {
	src := `<meta charset='utf-8'><span style="color: rgb(32, 33, 36); font-family: arial, sans-serif; font-size: 16px; font-style: normal; font-weight: 400; text-decoration: none; background-color: rgb(255, 255, 255);">Hello </span><b style="color: rgb(32, 33, 36); font-family: arial, sans-serif; font-size: 16px;">world</b>`
	want := `sz12+c202124+bgffffff"Hello " b+sz12+c202124"world"`
	if got := rdParse(src); got != want {
		t.Errorf("\n  получили %s\n  ждали    %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// Устойчивость

func TestRichHTML_DeepNestingDoesNotCrash(t *testing.T) {
	deep := strings.Repeat("<b>", 100000) + "x" + strings.Repeat("</b>", 100000)
	if got := rdParse(deep); got != `b"x"` {
		t.Errorf("%s", got)
	}
	deepDiv := strings.Repeat("<div>", 50000) + "x"
	if got := rdParse(deepDiv); got != `-"x"` {
		t.Errorf("%s", got)
	}
	// Вложенные списки не раздувают отступ: он ограничен.
	ps := RichParagraphsFromHTML(strings.Repeat("<ul>", 5000) + "<li>x")
	if len(ps) != 1 || ps[0].Indent > rhMaxIndent {
		t.Errorf("отступ %d", ps[0].Indent)
	}
}

func TestRichHTML_ManySpansLinear(t *testing.T) {
	// Много соседних одинаковых ранов не должно давать квадратичных копирований.
	src := "<p>" + strings.Repeat("<span>x</span>", 200000) + "</p>"
	start := time.Now()
	ps := RichParagraphsFromHTML(src)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("слишком долго: %v", d)
	}
	if len(ps) != 1 || richParaLen(ps[0]) != 200000 || len(ps[0].Runs) != 1 {
		t.Errorf("раны: %d", len(ps[0].Runs))
	}
}

func TestRichHTML_RandomGarbageDoesNotPanic(t *testing.T) {
	pieces := []string{
		"<", ">", "</", "/>", "<p>", "</p>", "<b>", "</b>", "<a href=\"", "\"", "'", "=", " ",
		"<br>", "<li>", "<ul>", "</ul>", "<!--", "-->", "<![", "]>", "<style>", "</style>",
		"<script>", "&", "&amp;", "&#", "&#x", ";", "style=\"color:#", "font-size:", "text", "ж", "😀",
		"\n", "\x00", "<span style=\"", "<td>", "<table>", "<pre>", "</pre>", "<head>", "<?", "<!",
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 5000; i++ {
		var sb strings.Builder
		for k := rng.Intn(40); k >= 0; k-- {
			sb.WriteString(pieces[rng.Intn(len(pieces))])
		}
		src := sb.String()
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("паника на %q: %v", src, r)
				}
			}()
			ps := RichParagraphsFromHTML(src)
			// Результат годится для документа.
			d := NewRichDocument(ps)
			rdCheck(t, d)
		}()
	}
}

// ---------------------------------------------------------------------------
// Круг: выгрузка в HTML → разбор

// rdExport выгружает абзацы так же, как копирование из виджета.
func rdExport(paras []RichParagraph) string {
	return richSelectionHTML(paras, richBuildDoc(paras), 0, richBuildDoc(paras).paraEnd(len(paras)-1))
}

func rdRoundTrip(t *testing.T, name string, src []RichParagraph) {
	t.Helper()
	d := NewRichDocument(src)
	want := d.Paragraphs()
	html := rdExport(want)
	got := RichParagraphsFromHTML(html)
	if !richParasEqual(want, got) {
		t.Errorf("%s: круг изменил документ\n  было     %s\n  стало    %s\n  html     %s",
			name, rdDumpParas(want), rdDumpParas(got), html)
	}
}

func TestRichHTML_RoundTrip(t *testing.T) {
	cases := map[string][]RichParagraph{
		"простой": {rdP(rdTxt("Hello", RichRun{}))},
		"стили": {rdP(
			rdTxt("plain ", RichRun{}), rdTxt("bold", rdBold), rdTxt(" ", RichRun{}),
			rdTxt("italic", rdItalic), rdTxt("bi", RichRun{Font: BuiltinFontBoldItalic}),
			rdTxt("mono", RichRun{Font: BuiltinFontMono}))},
		"цвет, фон, кегль": {rdP(
			rdTxt("a", RichRun{Color: color.RGBA{10, 20, 30, 255}}),
			rdTxt("b", RichRun{BG: color.RGBA{255, 255, 0, 255}, Size: 14.5}),
			rdTxt("c", RichRun{Size: 9}))},
		"подчёркивание и зачёркивание": {rdP(
			rdTxt("u", RichRun{Underline: true}), rdTxt("s", RichRun{Strike: true}),
			rdTxt("us", RichRun{Underline: true, Strike: true}))},
		"всё сразу": {rdP(rdTxt("x", RichRun{
			Font: BuiltinFontBoldItalic, Size: 18, Color: color.RGBA{1, 2, 3, 255},
			BG: color.RGBA{250, 251, 252, 255}, Underline: true, Strike: true, Link: "https://a.b/c?d=1&e=2",
		}))},
		"ссылка": {rdP(
			rdTxt("см. ", RichRun{}), rdTxt("сайт", RichRun{Link: "https://example.com"}), rdTxt(".", RichRun{}))},
		"ссылка жирная": {rdP(rdTxt("x", RichRun{Link: "http://a", Font: BuiltinFontBold}))},
		"выравнивание и отступы": {
			{Runs: []RichRun{rdTxt("a", RichRun{})}, Align: TextAlignCenter, Indent: 20, SpaceBefore: 3, SpaceAfter: 4},
			{Runs: []RichRun{rdTxt("b", RichRun{})}, Align: TextAlignRight},
			{Runs: []RichRun{rdTxt("c", RichRun{})}},
		},
		"пустые абзацы": {
			rdP(rdTxt("a", RichRun{})), rdP(), rdP(), rdP(rdTxt("b", RichRun{})),
		},
		"мягкий перевод строки":  {rdP(rdTxt("a\nb", RichRun{}), rdTxt("\nc", rdBold))},
		"мягкий перевод в конце": {rdP(rdTxt("a\n", RichRun{}))},
		"только мягкий перевод":  {rdP(rdTxt("\n", RichRun{}))},
		"два мягких подряд":      {rdP(rdTxt("a\n\nb", RichRun{}))},
		"спецсимволы":            {rdP(rdTxt(`<b>&amp; "q" 'a' </b>`, RichRun{}))},
		"кириллица и эмодзи":     {rdP(rdTxt("Привет, ", RichRun{}), rdTxt("мир 😀", rdBold)), rdP(rdTxt("日本語", rdItalic))},
		"пробелы": {
			rdP(rdTxt("  начало", RichRun{})),
			rdP(rdTxt("конец  ", RichRun{})),
			rdP(rdTxt("а  б   в", RichRun{})),
			rdP(rdTxt("   ", RichRun{})),
			rdP(rdTxt("a ", RichRun{}), rdTxt(" b", rdBold)),
			rdP(rdTxt("a \n b", RichRun{})),
			rdP(rdTxt(" ", RichRun{})),
		},
		"много абзацев разного вида": {
			{Runs: []RichRun{rdTxt("Заголовок", RichRun{Font: BuiltinFontBold, Size: 20})}, Align: TextAlignCenter, SpaceAfter: 8},
			{Runs: []RichRun{rdTxt("Текст с ", RichRun{}), rdTxt("ссылкой", RichRun{Link: "https://x.y"}), rdTxt(" и ", RichRun{}), rdTxt("цветом", RichRun{Color: color.RGBA{200, 0, 0, 255}})}, Indent: 12},
			rdP(),
			{Runs: []RichRun{rdTxt("• пункт", RichRun{})}, Indent: 24},
		},
	}
	for name, src := range cases {
		rdRoundTrip(t, name, src)
	}
}

func TestRichHTML_RoundTripPartialSelection(t *testing.T) {
	// Выгрузка части документа и разбор: совпадает с фрагментом.
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("Hello ", rdBold), rdTxt("World", RichRun{})}, Align: TextAlignCenter},
		rdP(rdTxt("мир ", rdItalic), rdTxt("😀!", RichRun{Underline: true})),
	})
	paras := d.Paragraphs()
	doc := richBuildDoc(paras)
	for lo := 0; lo <= d.Len(); lo++ {
		for hi := lo + 1; hi <= d.Len(); hi++ {
			html := richSelectionHTML(paras, doc, lo, hi)
			got := RichParagraphsFromHTML(html)
			want := d.Fragment(lo, hi)
			// Выгрузка не включает абзац, до начала которого выделение лишь
			// доходит (richSelectionHTML): пустой хвостовой абзац пропадает.
			if n := len(want); n > 1 && richParaLen(want[n-1]) == 0 && len(got) == n-1 {
				want = want[:n-1]
			}
			// Первая строка выделения, оканчивающаяся на краю абзаца, может не
			// включать пустой хвост — сравниваем текст и оформление рун.
			if richParasPlainEqual(want, got) {
				continue
			}
			t.Fatalf("[%d,%d): выгрузка\n  было  %s\n  стало %s\n  html  %s", lo, hi, rdDumpParas(want), rdDumpParas(got), html)
		}
	}
}

// richParasPlainEqual — равенство абзацев без учёта запомненного оформления
// пустых абзацев (оно в HTML не выгружается).
func richParasPlainEqual(a, b []RichParagraph) bool {
	norm := func(ps []RichParagraph) []RichParagraph {
		out := richCloneParas(ps)
		for i := range out {
			if richParaLen(out[i]) == 0 {
				out[i].Runs = nil
			}
		}
		return out
	}
	return richParasEqual(norm(a), norm(b))
}

func TestRichHTML_RoundTripSemiTransparentColor(t *testing.T) {
	// Прозрачность в CSS пишется с точностью до сотых, поэтому круг не
	// побитовый, но отклонение не более единицы на канал.
	src := []RichParagraph{rdP(rdTxt("x", RichRun{Color: color.RGBA{100, 50, 25, 200}}))}
	got := RichParagraphsFromHTML(rdExport(src))
	if len(got) != 1 || len(got[0].Runs) != 1 {
		t.Fatalf("%v", got)
	}
	a, b := src[0].Runs[0].Color, got[0].Runs[0].Color
	diff := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	if diff(a.R, b.R) > 1 || diff(a.G, b.G) > 1 || diff(a.B, b.B) > 1 || diff(a.A, b.A) > 1 {
		t.Errorf("цвет %v → %v", a, b)
	}
}

func TestRichHTML_RoundTripLosesOnlyCustomFont(t *testing.T) {
	// Имя собственного шрифта в HTML выгружается, но разбор его не переносит —
	// остальное оформление цело.
	src := []RichParagraph{rdP(rdTxt("x", RichRun{Font: "Моё Шрифт", Size: 12, Underline: true}))}
	got := RichParagraphsFromHTML(rdExport(src))
	want := []RichParagraph{rdP(rdTxt("x", RichRun{Size: 12, Underline: true}))}
	if !richParasEqual(want, got) {
		t.Errorf("%s", rdDumpParas(got))
	}
}

func TestRichHTML_ExportedLinkDangerousDroppedOnRoundTrip(t *testing.T) {
	src := []RichParagraph{rdP(rdTxt("x", RichRun{Link: "javascript:alert(1)"}), rdTxt("y", RichRun{Link: "https://ok"}))}
	got := RichParagraphsFromHTML(rdExport(src))
	want := []RichParagraph{rdP(rdTxt("x", RichRun{}), rdTxt("y", RichRun{Link: "https://ok"}))}
	if !richParasEqual(want, got) {
		t.Errorf("%s", rdDumpParas(got))
	}
}

func TestRichHTML_PasteIntoDocument(t *testing.T) {
	// Разобранный HTML вставляется в документ одной отменой.
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abXYcd", RichRun{}))})
	ps := RichParagraphsFromHTML(`<p>one <b>two</b></p><p>three</p>`)
	d.BeginGroup()
	d.Delete(2, 4)
	d.InsertParagraphs(2, ps)
	d.EndGroup()
	rdWant(t, d, `-"abone " b"two" ¶ -"threecd"`)
	d.Undo()
	rdWant(t, d, `-"abXYcd"`)
}
