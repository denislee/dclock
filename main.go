package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func main() {
	cfg, cfgPath, err := LoadConfig()
	if err != nil {
		log.Printf("config: %v (using defaults)", err)
	} else {
		log.Printf("config: %s", cfgPath)
	}
	InitFonts()

	go func() {
		if err := run(cfg, cfgPath); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type appState struct {
	cfg           Config
	cfgPath       string
	window        *app.Window
	parentCtx     context.Context
	weather       *WeatherClient
	weatherCancel context.CancelFunc
	settings      SettingsUI

	// showSeconds mirrors cfg.Clock.ShowSeconds for the ticker goroutine,
	// which must read it without holding any locks.
	showSeconds atomic.Bool
	// tickWake nudges the ticker goroutine to re-evaluate its interval
	// (e.g. after the user toggles ShowSeconds).
	tickWake chan struct{}

	// persistWake signals the persister goroutine that pendingCfg has been
	// updated. pendingCfg holds the most recent unsaved snapshot; it is
	// nil-able and guarded by pendingMu so the persister can read the
	// config without touching s.cfg (which is owned by the UI goroutine).
	persistWake chan struct{}
	pendingMu   sync.Mutex
	pendingCfg  *Config
}

const persistDebounce = 750 * time.Millisecond

func run(cfg Config, cfgPath string) error {
	var window app.Window
	window.Option(
		app.Title(cfg.Window.Title),
		app.Size(unit.Dp(cfg.Window.Width), unit.Dp(cfg.Window.Height)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := &appState{
		cfg:         cfg,
		cfgPath:     cfgPath,
		window:      &window,
		parentCtx:   ctx,
		tickWake:    make(chan struct{}, 1),
		persistWake: make(chan struct{}, 1),
	}
	st.showSeconds.Store(cfg.Clock.ShowSeconds)
	st.restartWeather()

	go st.runTicker(ctx)
	go st.runPersister(ctx)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(fontCollection))

	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			st.persistNow()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			p := ResolveTheme(st.cfg.Theme)
			th.Palette.Bg = p.Bg
			th.Palette.Fg = p.Fg
			th.Palette.ContrastBg = p.Accent
			th.Palette.ContrastFg = p.Fg

			fill(gtx, p.Bg)
			st.handleKeys(gtx)

			if st.settings.open {
				panelW := gtx.Dp(unit.Dp(panelWidthDp))
				if panelW > gtx.Constraints.Max.X-200 {
					panelW = gtx.Constraints.Max.X - 200
				}
				if panelW < 240 {
					panelW = gtx.Constraints.Max.X / 2
				}
				layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return drawMain(gtx, th, p, st, e.Now)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = panelW
						gtx.Constraints.Max.X = panelW
						gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
						before := st.cfg
						dims, wxChanged := st.settings.Layout(gtx, th, p, &st.cfg)
						if wxChanged {
							st.restartWeather()
						}
						if st.cfg != before {
							st.afterMutate()
						}
						return dims
					}),
				)
			} else {
				drawMain(gtx, th, p, st, e.Now)
			}

			e.Frame(gtx.Ops)
		}
	}
}

func (s *appState) handleKeys(gtx layout.Context) {
	event.Op(gtx.Ops, s)

	var filters []event.Filter
	if s.settings.open {
		filters = append(filters,
			key.Filter{Name: ","},
			key.Filter{Name: key.NameEscape},
		)
	} else {
		filters = append(filters,
			key.Filter{Name: "+"},
			key.Filter{Name: "="},
			key.Filter{Name: "-"},
			key.Filter{Name: "["},
			key.Filter{Name: "]"},
			key.Filter{Name: ","},
			key.Filter{Name: "S"},
			key.Filter{Name: "D"},
			key.Filter{Name: "F"},
			key.Filter{Name: "T"},
			key.Filter{Name: "Q"},
			key.Filter{Name: key.NameEscape},
		)
	}

	dirty := false
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case "+", "=":
			s.cfg.Clock.FontSize = clampFont(s.cfg.Clock.FontSize + 4)
			dirty = true
		case "-":
			s.cfg.Clock.FontSize = clampFont(s.cfg.Clock.FontSize - 4)
			dirty = true
		case "[":
			s.cfg.Weather.FontSize = clampWeatherFont(s.cfg.Weather.FontSize - 2)
			dirty = true
		case "]":
			s.cfg.Weather.FontSize = clampWeatherFont(s.cfg.Weather.FontSize + 2)
			dirty = true
		case ",":
			wasOpen := s.settings.open
			s.settings.Toggle(&s.cfg)
			if wasOpen {
				s.restartWeather()
			}
			dirty = true
		case "S":
			s.cfg.Clock.ShowSeconds = !s.cfg.Clock.ShowSeconds
			dirty = true
		case "D":
			s.cfg.Clock.ShowDate = !s.cfg.Clock.ShowDate
			dirty = true
		case "F":
			s.cfg.Clock.FontFace = nextFontFace(s.cfg.Clock.FontFace)
			dirty = true
		case "T":
			s.cfg.Theme = nextTheme(s.cfg.Theme)
			dirty = true
		case "Q":
			s.persistNow()
			s.window.Perform(system.ActionClose)
		case key.NameEscape:
			if s.settings.Close(&s.cfg) {
				s.restartWeather()
				dirty = true
			}
		}
	}
	if dirty {
		s.afterMutate()
	}
}

// afterMutate is called whenever cfg has changed in the UI thread. It keeps
// derived state (the ticker's ShowSeconds snapshot) in sync and schedules a
// debounced save.
func (s *appState) afterMutate() {
	if s.showSeconds.Swap(s.cfg.Clock.ShowSeconds) != s.cfg.Clock.ShowSeconds {
		select {
		case s.tickWake <- struct{}{}:
		default:
		}
	}
	s.markDirty()
}

func (s *appState) markDirty() {
	snap := s.cfg
	s.pendingMu.Lock()
	s.pendingCfg = &snap
	s.pendingMu.Unlock()
	select {
	case s.persistWake <- struct{}{}:
	default:
	}
}

func nextTheme(current string) string {
	idx := ThemeIndex(current)
	if idx < 0 {
		return Themes[0].Name
	}
	return Themes[(idx+1)%len(Themes)].Name
}

func nextFontFace(current string) string {
	if len(fontOptions) == 0 {
		return current
	}
	idx := FontIndex(current)
	if idx < 0 {
		return fontOptions[0].Name
	}
	return fontOptions[(idx+1)%len(fontOptions)].Name
}

func clampWeatherFont(v int) int {
	if v < 10 {
		return 10
	}
	if v > 48 {
		return 48
	}
	return v
}

func (s *appState) restartWeather() {
	if s.weatherCancel != nil {
		s.weatherCancel()
		s.weatherCancel = nil
	}
	s.weather = nil
	if !s.cfg.Weather.Enabled {
		return
	}
	wctx, wcancel := context.WithCancel(s.parentCtx)
	s.weatherCancel = wcancel
	s.weather = NewWeatherClient(s.cfg.Weather)
	go s.weather.Run(wctx, s.window.Invalidate)
}

// persistNow synchronously writes the current cfg. Called from the UI
// goroutine on shutdown paths (Quit, DestroyEvent), so reading s.cfg here is
// race-free. It also clears any pending snapshot so the persister's final
// flush won't redundantly rewrite the same data.
func (s *appState) persistNow() {
	s.pendingMu.Lock()
	s.pendingCfg = nil
	s.pendingMu.Unlock()
	if err := saveConfig(s.cfgPath, s.cfg); err != nil {
		log.Printf("save config: %v", err)
	}
}

// runTicker invalidates the window at the next clock boundary — every second
// when seconds are visible, otherwise every minute, aligned to wall-clock time
// so the displayed value lines up with the underlying second/minute change.
// It re-evaluates the interval whenever ShowSeconds changes via tickWake.
func (s *appState) runTicker(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		unit := time.Minute
		if s.showSeconds.Load() {
			unit = time.Second
		}
		now := time.Now()
		next := now.Truncate(unit).Add(unit)
		d := time.Until(next)
		if d < time.Millisecond {
			d = time.Millisecond
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.window.Invalidate()
		case <-s.tickWake:
			// Recompute interval on the next loop iteration.
		}
	}
}

// runPersister coalesces rapid config mutations into a single write per idle
// window. On shutdown it flushes any pending change before returning.
func (s *appState) runPersister(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	armed := false
	for {
		var timerC <-chan time.Time
		if armed {
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			s.flushPending()
			return
		case <-s.persistWake:
			if armed {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(persistDebounce)
			armed = true
		case <-timerC:
			armed = false
			s.flushPending()
		}
	}
}

// flushPending writes the most recent pending config snapshot to disk.
// Returns without doing anything if there's nothing pending.
func (s *appState) flushPending() {
	s.pendingMu.Lock()
	snap := s.pendingCfg
	s.pendingCfg = nil
	s.pendingMu.Unlock()
	if snap == nil {
		return
	}
	if err := saveConfig(s.cfgPath, *snap); err != nil {
		log.Printf("save config: %v", err)
	}
}

func fill(gtx layout.Context, c color.NRGBA) {
	rect := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	rect.Pop()
}

func drawMain(gtx layout.Context, th *material.Theme, p Palette, st *appState, now time.Time) layout.Dimensions {
	// The main area paints its own bg again so it stays correct under the slider.
	fill(gtx, p.Bg)
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return clockLayout(gtx, th, p, st.cfg.Clock, now)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !st.cfg.Weather.Enabled {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
				}
				return forecastLayout(gtx, th, p, st.cfg.Weather, st.weather)
			}),
		)
	})
}

func clockLayout(gtx layout.Context, th *material.Theme, p Palette, cc ClockConfig, now time.Time) layout.Dimensions {
	timeStr := formatTime(now, cc)

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(float32(cc.FontSize)), timeStr)
			lbl.Alignment = text.Middle
			lbl.Color = p.Fg
			lbl.Font.Weight = font.SemiBold
			lbl.Font.Typeface = ResolveFont(cc.FontFace)
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !cc.ShowDate {
				return layout.Dimensions{}
			}
			lbl := material.Label(th, unit.Sp(20), now.Format("Monday, 2 January 2006"))
			lbl.Alignment = text.Middle
			lbl.Color = p.Subtle
			lbl.Font.Typeface = ResolveFont(cc.FontFace)
			return lbl.Layout(gtx)
		}),
	)
}

func forecastLayout(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, client *WeatherClient) layout.Dimensions {
	if client == nil {
		return centerCaption(gtx, th, p, "Weather disabled")
	}
	w, err, _ := client.Snapshot()
	if w == nil && err != nil {
		return centerCaption(gtx, th, p, fmt.Sprintf("%s — weather unavailable", wc.LocationName))
	}
	if w == nil {
		return centerCaption(gtx, th, p, fmt.Sprintf("%s — loading…", wc.LocationName))
	}

	switch wc.View {
	case "hourly":
		return hourlyView(gtx, th, p, wc, w)
	case "daily":
		return dailyView(gtx, th, p, wc, w)
	}
	return currentView(gtx, th, p, wc, w)
}

func currentView(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, w *Weather) layout.Dimensions {
	line := fmt.Sprintf("%s  %s  %s  %s  •  rain %d%%  •  wind %.0f km/h",
		wc.LocationName, w.Icon, w.Temp(wc.Units), w.Description, w.PrecipPct, w.WindKMH)
	lbl := material.Label(th, wxSize(wc, 2), line)
	lbl.Alignment = text.Middle
	lbl.Color = p.Fg
	return layout.Center.Layout(gtx, lbl.Layout)
}

func hourlyView(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, w *Weather) layout.Dimensions {
	if len(w.HourlyCells) == 0 {
		return centerCaption(gtx, th, p, fmt.Sprintf("%s — no hourly data", wc.LocationName))
	}
	return forecastStrip(gtx, th, p, wc, fmt.Sprintf("%s — next 24 hours", wc.LocationName), w.HourlyCells)
}

func dailyView(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, w *Weather) layout.Dimensions {
	if len(w.DailyCells) == 0 {
		return centerCaption(gtx, th, p, fmt.Sprintf("%s — no daily data", wc.LocationName))
	}
	return forecastStrip(gtx, th, p, wc, fmt.Sprintf("%s — 7-day forecast", wc.LocationName), w.DailyCells)
}

func forecastStrip(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, header string, cells []ForecastCell) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, wxSize(wc, -2), header)
			lbl.Color = p.Subtle
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			row := layout.Flex{}
			children := make([]layout.FlexChild, len(cells))
			for i := range cells {
				c := cells[i]
				children[i] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return forecastCell(gtx, th, p, wc, c.Top, c.Icon, c.Primary, c.Secondary)
				})
			}
			return row.Layout(gtx, children...)
		}),
	)
}

func forecastCell(gtx layout.Context, th *material.Theme, p Palette, wc WeatherConfig, top, icon, primary, secondary string) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, wxSize(wc, -4), top)
				lbl.Color = p.Subtle
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, wxSize(wc, 6), icon)
				lbl.Alignment = text.Middle
				lbl.Color = p.Fg
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, wxSize(wc, 0), primary)
				lbl.Color = p.Fg
				lbl.Alignment = text.Middle
				lbl.Font.Weight = font.SemiBold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if secondary == "" {
					return layout.Dimensions{}
				}
				lbl := material.Label(th, wxSize(wc, -4), secondary)
				lbl.Color = p.Subtle
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			}),
		)
	})
}

func centerCaption(gtx layout.Context, th *material.Theme, p Palette, txt string) layout.Dimensions {
	lbl := material.Label(th, unit.Sp(16), txt)
	lbl.Alignment = text.Middle
	lbl.Color = p.Subtle
	return layout.Center.Layout(gtx, lbl.Layout)
}

func wxSize(wc WeatherConfig, delta int) unit.Sp {
	s := wc.FontSize + delta
	if wc.FontSize == 0 {
		s = 16 + delta
	}
	if s < 8 {
		s = 8
	}
	return unit.Sp(s)
}

func formatTime(t time.Time, cc ClockConfig) string {
	if cc.Format24h {
		if cc.ShowSeconds {
			return t.Format("15:04:05")
		}
		return t.Format("15:04")
	}
	if cc.ShowSeconds {
		return t.Format("3:04:05 PM")
	}
	return t.Format("3:04 PM")
}
