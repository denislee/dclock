package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ClockConfig struct {
	Format24h   bool   `json:"format_24h"`
	ShowSeconds bool   `json:"show_seconds"`
	ShowDate    bool   `json:"show_date"`
	FontSize    int    `json:"font_size"`
	FontFace    string `json:"font_face"`
}

type WeatherConfig struct {
	Enabled        bool    `json:"enabled"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	LocationName   string  `json:"location_name"`
	Units          string  `json:"units"`
	RefreshMinutes int     `json:"refresh_minutes"`
	View           string  `json:"view"` // "current" | "hourly" | "daily"
	FontSize       int     `json:"font_size"`
}

type WindowConfig struct {
	Title  string `json:"title"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Config struct {
	Clock   ClockConfig   `json:"clock"`
	Weather WeatherConfig `json:"weather"`
	Window  WindowConfig  `json:"window"`
	Theme   string        `json:"theme"`
}

func defaultConfig() Config {
	return Config{
		Clock: ClockConfig{
			Format24h:   true,
			ShowSeconds: true,
			ShowDate:    true,
			FontSize:    96,
			FontFace:    "Go",
		},
		Weather: WeatherConfig{
			Enabled:        true,
			Latitude:       50.4501,
			Longitude:      30.5234,
			LocationName:   "Kyiv",
			Units:          "celsius",
			RefreshMinutes: 30,
			View:           "current",
			FontSize:       16,
		},
		Window: WindowConfig{
			Title:  "dclock",
			Width:  640,
			Height: 360,
		},
		Theme: "Midnight",
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dclock", "config.json"), nil
}

func LoadConfig() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := defaultConfig()
		if werr := saveConfig(path, cfg); werr != nil {
			return cfg, path, werr
		}
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, path, err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, path, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, path, nil
}

func saveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
