package engine

// text_alias.go — прежние имена пакета engine, перенесённые в core/text (задача 4.1
// плана 4.0): код потребителей не меняется.

import hgtext "github.com/oops1/headless-gui/v3/core/text"

// DefaultDPI — см. text.DefaultDPI.
const DefaultDPI = hgtext.DefaultDPI

// DefaultFontSize — см. text.DefaultFontSize.
const DefaultFontSize = hgtext.DefaultFontSize

// DefaultTextContrast — см. text.DefaultTextContrast.
const DefaultTextContrast = hgtext.DefaultTextContrast

// FontCache — см. text.FontCache.
type FontCache = hgtext.FontCache

// LCDOrder — см. text.LCDOrder.
type LCDOrder = hgtext.LCDOrder

// LCDOrderBGR — см. text.LCDOrderBGR.
const LCDOrderBGR = hgtext.LCDOrderBGR

// LCDOrderRGB — см. text.LCDOrderRGB.
const LCDOrderRGB = hgtext.LCDOrderRGB

// TextQuality — см. text.TextQuality.
type TextQuality = hgtext.TextQuality
