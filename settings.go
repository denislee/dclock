package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// SettingsUI is the right-side slider panel.
type SettingsUI struct {
	open bool

	fmt24       widget.Bool
	showSeconds widget.Bool
	showDate    widget.Bool
	wxEnabled   widget.Bool
	useFahr     widget.Bool

	refresh widget.Editor

	// Dropdowns are accordion-style: clicking the trigger reveals the items.
	locTrigger   widget.Clickable
	locOpen      bool
	locItems     []widget.Clickable
	themeTrigger widget.Clickable
	themeOpen    bool
	themeItems   []widget.Clickable
	fontTrigger  widget.Clickable
	fontOpen     bool
	fontItems    []widget.Clickable

	viewCurrent widget.Clickable
	viewHourly  widget.Clickable
	viewDaily   widget.Clickable

	fontInc  widget.Clickable
	fontDec  widget.Clickable
	wxInc    widget.Clickable
	wxDec    widget.Clickable
	closeBtn widget.Clickable

	list   widget.List
	synced bool
}

const panelWidthDp = 360

func (s *SettingsUI) ensureInit() {
	if len(s.locItems) != len(Locations) {
		s.locItems = make([]widget.Clickable, len(Locations))
	}
	if len(s.themeItems) != len(Themes) {
		s.themeItems = make([]widget.Clickable, len(Themes))
	}
	if len(s.fontItems) != len(fontOptions) {
		s.fontItems = make([]widget.Clickable, len(fontOptions))
	}
	s.list.Axis = layout.Vertical
}

func (s *SettingsUI) Toggle(cfg *Config) {
	s.open = !s.open
	if s.open {
		s.syncFrom(cfg)
	} else {
		s.applyTo(cfg)
	}
}

func (s *SettingsUI) Close(cfg *Config) bool {
	if !s.open {
		return false
	}
	s.applyTo(cfg)
	s.open = false
	return true
}

func (s *SettingsUI) syncFrom(cfg *Config) {
	s.fmt24.Value = cfg.Clock.Format24h
	s.showSeconds.Value = cfg.Clock.ShowSeconds
	s.showDate.Value = cfg.Clock.ShowDate
	s.wxEnabled.Value = cfg.Weather.Enabled
	s.useFahr.Value = strings.EqualFold(cfg.Weather.Units, "fahrenheit")
	s.refresh.SingleLine = true
	if s.refresh.Text() != strconv.Itoa(cfg.Weather.RefreshMinutes) {
		s.refresh.SetText(strconv.Itoa(cfg.Weather.RefreshMinutes))
	}
	s.synced = true
}

func (s *SettingsUI) applyTo(cfg *Config) bool {
	if !s.synced {
		return false
	}
	prev := cfg.Weather
	cfg.Clock.Format24h = s.fmt24.Value
	cfg.Clock.ShowSeconds = s.showSeconds.Value
	cfg.Clock.ShowDate = s.showDate.Value
	cfg.Weather.Enabled = s.wxEnabled.Value
	if s.useFahr.Value {
		cfg.Weather.Units = "fahrenheit"
	} else {
		cfg.Weather.Units = "celsius"
	}
	if v, err := strconv.Atoi(strings.TrimSpace(s.refresh.Text())); err == nil && v > 0 {
		cfg.Weather.RefreshMinutes = v
	}
	return cfg.Weather != prev
}

// Layout renders the panel into the area allocated by gtx.Constraints.
// Returns true if a weather-affecting setting changed this frame.
func (s *SettingsUI) Layout(gtx layout.Context, th *material.Theme, p Palette, cfg *Config) (layout.Dimensions, bool) {
	s.ensureInit()

	size := gtx.Constraints.Max
	// Card background
	{
		r := clip.Rect{Max: size}.Push(gtx.Ops)
		paint.ColorOp{Color: p.Card}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		r.Pop()
	}
	// Left border
	{
		r := clip.Rect{Max: image.Pt(gtx.Dp(1), size.Y)}.Push(gtx.Ops)
		paint.ColorOp{Color: p.Border}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		r.Pop()
	}

	prevWx := cfg.Weather
	prevTheme := cfg.Theme
	prevFont := cfg.Clock.FontFace

	// Buttons / inputs that mutate state
	if s.fontInc.Clicked(gtx) {
		cfg.Clock.FontSize = clampFont(cfg.Clock.FontSize + 8)
	}
	if s.fontDec.Clicked(gtx) {
		cfg.Clock.FontSize = clampFont(cfg.Clock.FontSize - 8)
	}
	if s.wxInc.Clicked(gtx) {
		cfg.Weather.FontSize = clampWeatherFont(cfg.Weather.FontSize + 2)
	}
	if s.wxDec.Clicked(gtx) {
		cfg.Weather.FontSize = clampWeatherFont(cfg.Weather.FontSize - 2)
	}
	if s.closeBtn.Clicked(gtx) {
		s.Close(cfg)
	}
	if s.locTrigger.Clicked(gtx) {
		s.locOpen = !s.locOpen
	}
	if s.themeTrigger.Clicked(gtx) {
		s.themeOpen = !s.themeOpen
	}
	if s.fontTrigger.Clicked(gtx) {
		s.fontOpen = !s.fontOpen
	}
	for i := range s.locItems {
		if s.locItems[i].Clicked(gtx) {
			cfg.Weather.LocationName = Locations[i].Name
			cfg.Weather.Latitude = Locations[i].Latitude
			cfg.Weather.Longitude = Locations[i].Longitude
			s.locOpen = false
		}
	}
	for i := range s.themeItems {
		if s.themeItems[i].Clicked(gtx) {
			cfg.Theme = Themes[i].Name
			s.themeOpen = false
		}
	}
	for i := range s.fontItems {
		if s.fontItems[i].Clicked(gtx) {
			cfg.Clock.FontFace = fontOptions[i].Name
			s.fontOpen = false
		}
	}
	if s.viewCurrent.Clicked(gtx) {
		cfg.Weather.View = "current"
	}
	if s.viewHourly.Clicked(gtx) {
		cfg.Weather.View = "hourly"
	}
	if s.viewDaily.Clicked(gtx) {
		cfg.Weather.View = "daily"
	}

	// Live-apply toggles for instant preview
	cfg.Clock.Format24h = s.fmt24.Value
	cfg.Clock.ShowSeconds = s.showSeconds.Value
	cfg.Clock.ShowDate = s.showDate.Value
	cfg.Weather.Enabled = s.wxEnabled.Value
	if s.useFahr.Value {
		cfg.Weather.Units = "fahrenheit"
	} else {
		cfg.Weather.Units = "celsius"
	}

	rows := buildRows(s, th, p, cfg)
	inner := func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &s.list).Layout(gtx, len(rows), func(gtx layout.Context, idx int) layout.Dimensions {
				return rows[idx](gtx)
			})
		})
	}
	inner(gtx)

	wxChanged := cfg.Weather != prevWx
	themeChanged := cfg.Theme != prevTheme
	fontChanged := cfg.Clock.FontFace != prevFont
	return layout.Dimensions{Size: size}, wxChanged || themeChanged || fontChanged
}

func buildRows(s *SettingsUI, th *material.Theme, p Palette, cfg *Config) []layout.Widget {
	spacer := func(h unit.Dp) layout.Widget {
		return layout.Spacer{Height: h}.Layout
	}

	rows := []layout.Widget{
		title(th, p, "Settings"),
		spacer(12),
		section(th, p, "Clock"),
		spacer(6),
		toggleRow(th, p, &s.fmt24, "24-hour clock"),
		toggleRow(th, p, &s.showSeconds, "Show seconds"),
		toggleRow(th, p, &s.showDate, "Show date"),
		fontRow(th, p, s, cfg.Clock.FontSize),
		spacer(6),
		caption(th, p, "Font face"),
		dropdownTrigger(th, p, &s.fontTrigger, fontFaceDisplay(cfg.Clock.FontFace), s.fontOpen),
	}
	if s.fontOpen {
		for i, fo := range fontOptions {
			selected := equalFold(fo.Name, cfg.Clock.FontFace)
			rows = append(rows, dropdownItem(th, p, &s.fontItems[i], fo.Name, selected))
		}
	}
	rows = append(rows,
		spacer(14),
		dividerRow(p),
		spacer(14),
		section(th, p, "Weather"),
		spacer(6),
		toggleRow(th, p, &s.wxEnabled, "Enabled"),
		toggleRow(th, p, &s.useFahr, "Fahrenheit (off = Celsius)"),
		wxFontRow(th, p, s, cfg.Weather.FontSize),
		spacer(6),
		caption(th, p, "Location"),
		dropdownTrigger(th, p, &s.locTrigger, cfg.Weather.LocationName, s.locOpen),
	)
	if s.locOpen {
		for i, loc := range Locations {
			selected := equalFold(loc.Name, cfg.Weather.LocationName)
			rows = append(rows, dropdownItem(th, p, &s.locItems[i], loc.Name, selected))
		}
	}
	rows = append(rows,
		spacer(8),
		caption(th, p, "Forecast view"),
		viewRow(th, p, s, cfg.Weather.View),
		spacer(8),
		caption(th, p, "Refresh (minutes)"),
		editorRow(th, p, &s.refresh),
		spacer(14),
		dividerRow(p),
		spacer(14),
		section(th, p, "Appearance"),
		spacer(6),
		caption(th, p, "Theme"),
		dropdownTrigger(th, p, &s.themeTrigger, cfg.Theme, s.themeOpen),
	)
	if s.themeOpen {
		for i, t := range Themes {
			selected := equalFold(t.Name, cfg.Theme)
			rows = append(rows, dropdownItem(th, p, &s.themeItems[i], t.Name, selected))
		}
	}
	rows = append(rows,
		spacer(18),
		func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, &s.closeBtn, "Apply & close")
			btn.Background = p.Accent
			btn.Color = contrastOn(p.Accent)
			return btn.Layout(gtx)
		},
		spacer(10),
		captionMuted(th, p, "Shortcuts:  + / -  clock font  •  [ / ]  forecast font  •  s seconds  •  d date  •  f font face  •  t theme  •  , settings  •  q quit  •  Esc close"),
	)
	return rows
}

func title(th *material.Theme, p Palette, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.H5(th, txt)
		l.Color = p.Fg
		l.Font.Weight = font.SemiBold
		return l.Layout(gtx)
	}
}

func section(th *material.Theme, p Palette, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Body1(th, strings.ToUpper(txt))
		l.Color = p.Subtle
		l.Font.Weight = font.SemiBold
		l.TextSize = unit.Sp(11)
		return l.Layout(gtx)
	}
}

func caption(th *material.Theme, p Palette, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(th, txt)
		l.Color = p.Subtle
		return l.Layout(gtx)
	}
}

func captionMuted(th *material.Theme, p Palette, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(th, txt)
		l.Color = withAlpha(p.Subtle, 0xc0)
		return l.Layout(gtx)
	}
}

func toggleRow(th *material.Theme, p Palette, b *widget.Bool, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sw := material.Switch(th, b, label)
					sw.Color.Enabled = p.Accent
					sw.Color.Disabled = p.Border
					sw.Color.Track = withAlpha(p.Subtle, 0x55)
					return sw.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body1(th, label)
					l.Color = p.Fg
					return l.Layout(gtx)
				}),
			)
		})
	}
}

func wxFontRow(th *material.Theme, p Palette, s *SettingsUI, size int) layout.Widget {
	if size == 0 {
		size = 16
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body1(th, "Forecast size: "+strconv.Itoa(size))
					l.Color = p.Fg
					return l.Layout(gtx)
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(themedButton(th, p, &s.wxDec, " − ")),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(themedButton(th, p, &s.wxInc, " + ")),
			)
		})
	}
}

func fontRow(th *material.Theme, p Palette, s *SettingsUI, size int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body1(th, "Font size: "+strconv.Itoa(size))
					l.Color = p.Fg
					return l.Layout(gtx)
				}),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(themedButton(th, p, &s.fontDec, " − ")),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(themedButton(th, p, &s.fontInc, " + ")),
			)
		})
	}
}

func viewRow(th *material.Theme, p Palette, s *SettingsUI, current string) layout.Widget {
	options := []struct {
		key  string
		text string
		btn  *widget.Clickable
	}{
		{"current", "Now", &s.viewCurrent},
		{"hourly", "24h", &s.viewHourly},
		{"daily", "7-day", &s.viewDaily},
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(options)*2)
			for i, opt := range options {
				if i > 0 {
					children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
				}
				selected := opt.key == current
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					b := material.Button(th, opt.btn, opt.text)
					if selected {
						b.Background = p.Accent
						b.Color = contrastOn(p.Accent)
					} else {
						b.Background = p.Input
						b.Color = p.Fg
					}
					b.CornerRadius = unit.Dp(4)
					b.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}
					return b.Layout(gtx)
				}))
			}
			return layout.Flex{}.Layout(gtx, children...)
		})
	}
}

func themedButton(th *material.Theme, p Palette, c *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		b := material.Button(th, c, label)
		b.Background = p.Input
		b.Color = p.Fg
		b.CornerRadius = unit.Dp(4)
		b.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}
		return b.Layout(gtx)
	}
}

func dropdownTrigger(th *material.Theme, p Palette, c *widget.Clickable, current string, expanded bool) layout.Widget {
	label := current
	if expanded {
		label += "  ▴"
	} else {
		label += "  ▾"
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, c, label)
			b.Background = p.Input
			b.Color = p.Fg
			b.CornerRadius = unit.Dp(4)
			b.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}
			return b.Layout(gtx)
		})
	}
}

func dropdownItem(th *material.Theme, p Palette, c *widget.Clickable, label string, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := p.Card
			if c.Hovered() {
				bg = withAlpha(p.Accent, 0x33)
			}
			if selected {
				bg = withAlpha(p.Accent, 0x55)
			}
			r := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(34))}.Push(gtx.Ops)
			paint.ColorOp{Color: bg}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			r.Pop()
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, label)
				l.Color = p.Fg
				if selected {
					l.Font.Weight = font.SemiBold
				}
				return l.Layout(gtx)
			})
		})
	}
}

func editorRow(th *material.Theme, p Palette, ed *widget.Editor) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			ed.SingleLine = true
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					sz := image.Pt(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
					r := clip.Rect{Max: sz}.Push(gtx.Ops)
					paint.ColorOp{Color: p.Input}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					r.Pop()
					return layout.Dimensions{Size: sz}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						e := material.Editor(th, ed, "")
						e.TextSize = unit.Sp(14)
						e.Color = p.Fg
						e.HintColor = withAlpha(p.Subtle, 0x80)
						e.Editor.Alignment = text.Start
						return e.Layout(gtx)
					})
				},
			)
		})
	}
}

func dividerRow(p Palette) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(unit.Dp(1))
		size := image.Pt(gtx.Constraints.Max.X, h)
		r := clip.Rect{Max: size}.Push(gtx.Ops)
		paint.ColorOp{Color: p.Border}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		r.Pop()
		return layout.Dimensions{Size: size}
	}
}

func fontFaceDisplay(name string) string {
	if name == "" {
		return "Default"
	}
	return name
}

func clampFont(v int) int {
	if v < 24 {
		return 24
	}
	if v > 240 {
		return 240
	}
	return v
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// contrastOn returns black or white depending on what reads better on c.
func contrastOn(c color.NRGBA) color.NRGBA {
	// perceived luminance
	y := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	if y > 160 {
		return color.NRGBA{A: 0xff}
	}
	return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
}
