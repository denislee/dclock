package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Weather struct {
	TempC       float64
	TempF       float64
	WindKMH     float64
	Code        int
	Description string
	Icon        string
	PrecipPct   int // chance of precipitation for the current hour
	Time        time.Time

	Hourly []HourlyPoint
	Daily  []DailyPoint

	// Pre-formatted display rows so the UI doesn't re-derive them every frame.
	HourlyCells []ForecastCell
	DailyCells  []ForecastCell
}

// ForecastCell is a ready-to-render hourly or daily cell.
type ForecastCell struct {
	Top, Icon, Primary, Secondary string
}

type HourlyPoint struct {
	Time      time.Time
	TempC     float64
	TempF     float64
	Code      int
	Icon      string
	PrecipPct int
}

type DailyPoint struct {
	Date        time.Time
	HiC, LoC    float64
	HiF, LoF    float64
	Code        int
	Icon        string
	Description string
	PrecipPct   int // daily max chance of precipitation
}

func (w Weather) Temp(units string) string {
	if units == "fahrenheit" {
		return fmt.Sprintf("%.0f°F", w.TempF)
	}
	return fmt.Sprintf("%.0f°C", w.TempC)
}

func (h HourlyPoint) Temp(units string) string {
	if units == "fahrenheit" {
		return fmt.Sprintf("%.0f°", h.TempF)
	}
	return fmt.Sprintf("%.0f°", h.TempC)
}

func (d DailyPoint) Hi(units string) string {
	if units == "fahrenheit" {
		return fmt.Sprintf("%.0f°", d.HiF)
	}
	return fmt.Sprintf("%.0f°", d.HiC)
}

func (d DailyPoint) Lo(units string) string {
	if units == "fahrenheit" {
		return fmt.Sprintf("%.0f°", d.LoF)
	}
	return fmt.Sprintf("%.0f°", d.LoC)
}

// httpClient is shared across fetches so connections (and TLS handshakes)
// can be reused between weather refreshes.
var httpClient = &http.Client{Timeout: 15 * time.Second}

type WeatherClient struct {
	mu      sync.RWMutex
	cfg     WeatherConfig
	last    *Weather
	lastErr error
	fetched time.Time
}

func NewWeatherClient(cfg WeatherConfig) *WeatherClient {
	return &WeatherClient{cfg: cfg}
}

func (c *WeatherClient) Snapshot() (*Weather, error, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.last, c.lastErr, c.fetched
}

func (c *WeatherClient) Run(ctx context.Context, notify func()) {
	interval := time.Duration(c.cfg.RefreshMinutes) * time.Minute
	if interval < time.Minute {
		interval = 30 * time.Minute
	}
	c.fetchOnce(ctx, notify)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.fetchOnce(ctx, notify)
		}
	}
}

func (c *WeatherClient) fetchOnce(ctx context.Context, notify func()) {
	w, err := fetchWeather(ctx, c.cfg)
	c.mu.Lock()
	c.last = w
	c.lastErr = err
	c.fetched = time.Now()
	c.mu.Unlock()
	if notify != nil {
		notify()
	}
}

func fetchWeather(ctx context.Context, cfg WeatherConfig) (*Weather, error) {
	tempUnit := "celsius"
	if cfg.Units == "fahrenheit" {
		tempUnit = "fahrenheit"
	}
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(cfg.Latitude, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(cfg.Longitude, 'f', 4, 64))
	q.Set("current", "temperature_2m,weather_code,wind_speed_10m")
	q.Set("hourly", "temperature_2m,weather_code,precipitation_probability")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max")
	q.Set("forecast_days", "7")
	q.Set("temperature_unit", tempUnit)
	q.Set("wind_speed_unit", "kmh")
	q.Set("timezone", "auto")

	endpoint := "https://api.open-meteo.com/v1/forecast?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("weather: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			WeatherCode int     `json:"weather_code"`
			WindSpeed   float64 `json:"wind_speed_10m"`
		} `json:"current"`
		Hourly struct {
			Time          []string  `json:"time"`
			Temperature   []float64 `json:"temperature_2m"`
			WeatherCode   []int     `json:"weather_code"`
			PrecipProb    []int     `json:"precipitation_probability"`
		} `json:"hourly"`
		Daily struct {
			Time            []string  `json:"time"`
			WeatherCode     []int     `json:"weather_code"`
			Hi              []float64 `json:"temperature_2m_max"`
			Lo              []float64 `json:"temperature_2m_min"`
			PrecipProbMax   []int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	w := &Weather{
		Code:    payload.Current.WeatherCode,
		WindKMH: payload.Current.WindSpeed,
		Time:    time.Now(),
	}
	if tempUnit == "fahrenheit" {
		w.TempF = payload.Current.Temperature
		w.TempC = (w.TempF - 32) * 5 / 9
	} else {
		w.TempC = payload.Current.Temperature
		w.TempF = w.TempC*9/5 + 32
	}
	w.Description, w.Icon = describeWMO(w.Code)

	now := time.Now()
	// Hourly: keep the next 24 entries starting from the current hour.
	for i := range payload.Hourly.Time {
		t, err := time.ParseInLocation("2006-01-02T15:04", payload.Hourly.Time[i], time.Local)
		if err != nil {
			continue
		}
		if t.Before(now.Truncate(time.Hour)) {
			continue
		}
		hp := HourlyPoint{Time: t, Code: payload.Hourly.WeatherCode[i]}
		if i < len(payload.Hourly.PrecipProb) {
			hp.PrecipPct = payload.Hourly.PrecipProb[i]
		}
		if tempUnit == "fahrenheit" {
			hp.TempF = payload.Hourly.Temperature[i]
			hp.TempC = (hp.TempF - 32) * 5 / 9
		} else {
			hp.TempC = payload.Hourly.Temperature[i]
			hp.TempF = hp.TempC*9/5 + 32
		}
		_, hp.Icon = describeWMO(hp.Code)
		w.Hourly = append(w.Hourly, hp)
		if len(w.Hourly) >= 24 {
			break
		}
	}
	// Use the first hourly entry's precipitation probability as the current chance.
	if len(w.Hourly) > 0 {
		w.PrecipPct = w.Hourly[0].PrecipPct
	}

	for i := range payload.Daily.Time {
		t, err := time.ParseInLocation("2006-01-02", payload.Daily.Time[i], time.Local)
		if err != nil {
			continue
		}
		dp := DailyPoint{Date: t, Code: payload.Daily.WeatherCode[i]}
		if i < len(payload.Daily.PrecipProbMax) {
			dp.PrecipPct = payload.Daily.PrecipProbMax[i]
		}
		if tempUnit == "fahrenheit" {
			dp.HiF = payload.Daily.Hi[i]
			dp.LoF = payload.Daily.Lo[i]
			dp.HiC = (dp.HiF - 32) * 5 / 9
			dp.LoC = (dp.LoF - 32) * 5 / 9
		} else {
			dp.HiC = payload.Daily.Hi[i]
			dp.LoC = payload.Daily.Lo[i]
			dp.HiF = dp.HiC*9/5 + 32
			dp.LoF = dp.LoC*9/5 + 32
		}
		dp.Description, dp.Icon = describeWMO(dp.Code)
		w.Daily = append(w.Daily, dp)
		if len(w.Daily) >= 7 {
			break
		}
	}

	w.buildCells(cfg.Units)
	return w, nil
}

// buildCells precomputes the display strings used by the hourly and daily
// views, so the layout pass doesn't allocate them every frame.
func (w *Weather) buildCells(units string) {
	const hourlyStep = 3
	const hourlyMax = 8
	w.HourlyCells = w.HourlyCells[:0]
	for i := 0; i < len(w.Hourly) && len(w.HourlyCells) < hourlyMax; i += hourlyStep {
		hp := w.Hourly[i]
		w.HourlyCells = append(w.HourlyCells, ForecastCell{
			Top:       hp.Time.Format("15:04"),
			Icon:      hp.Icon,
			Primary:   hp.Temp(units),
			Secondary: fmt.Sprintf("%d%% ☂", hp.PrecipPct),
		})
	}
	w.DailyCells = w.DailyCells[:0]
	for _, dp := range w.Daily {
		w.DailyCells = append(w.DailyCells, ForecastCell{
			Top:       dp.Date.Format("Mon"),
			Icon:      dp.Icon,
			Primary:   fmt.Sprintf("%s / %s", dp.Hi(units), dp.Lo(units)),
			Secondary: fmt.Sprintf("%d%% ☂", dp.PrecipPct),
		})
	}
}

// describeWMO maps WMO weather codes (Open-Meteo) to a short label + glyph.
// Icons use BMP-only Unicode symbols so they render in common system fonts
// (DejaVu Sans, Liberation Sans, Noto Sans, etc.) without needing a colour
// emoji font.
// https://open-meteo.com/en/docs
func describeWMO(code int) (string, string) {
	switch code {
	case 0:
		return "Clear", "☀" // ☀
	case 1:
		return "Mostly clear", "☼" // ☼
	case 2:
		return "Partly cloudy", "☼☁"
	case 3:
		return "Overcast", "☁" // ☁
	case 45, 48:
		return "Fog", "▒" // ▒
	case 51, 53, 55:
		return "Drizzle", "☂" // ☂
	case 56, 57:
		return "Freezing drizzle", "☂" // ☂
	case 61, 63, 65:
		return "Rain", "☔" // ☔
	case 66, 67:
		return "Freezing rain", "☔" // ☔
	case 71, 73, 75:
		return "Snow", "❄" // ❄
	case 77:
		return "Snow grains", "❄" // ❄
	case 80, 81, 82:
		return "Rain showers", "☔" // ☔
	case 85, 86:
		return "Snow showers", "❄" // ❄
	case 95:
		return "Thunderstorm", "⚡" // ⚡
	case 96, 99:
		return "Thunderstorm w/ hail", "⚡" // ⚡
	}
	return "Unknown", "•" // •
}
