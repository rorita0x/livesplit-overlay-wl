package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Splits     string   `toml:"splits"`
	Layout     string   `toml:"layout"`
	Width      int      `toml:"width"`
	Height     int      `toml:"height"`
	Corner     string   `toml:"corner"`
	Margin     int      `toml:"margin"`
	Opacity    float64  `toml:"opacity"`
	Outputs    []string `toml:"outputs"`
	OverPanels bool     `toml:"over_panels"`
	FPS        int      `toml:"fps"`
	Autosave   bool     `toml:"autosave"`
	Language   string   `toml:"language"`
	Hotkeys    Hotkeys  `toml:"hotkeys"`
}

type Hotkeys struct {
	Enabled            bool   `toml:"enabled"`
	Split              string `toml:"split"`
	Reset              string `toml:"reset"`
	Undo               string `toml:"undo"`
	Skip               string `toml:"skip"`
	Pause              string `toml:"pause"`
	UndoAllPauses      string `toml:"undo_all_pauses"`
	PreviousComparison string `toml:"previous_comparison"`
	NextComparison     string `toml:"next_comparison"`
	ToggleTimingMethod string `toml:"toggle_timing_method"`
}

type Binding struct {
	Action string
	Key    string
}

func Default() Config {
	return Config{
		Width:    300,
		Height:   500,
		Corner:   "top-right",
		Margin:   12,
		Opacity:  1,
		FPS:      30,
		Autosave: true,
		Hotkeys: Hotkeys{
			Enabled:            true,
			Split:              "Numpad1",
			Reset:              "Numpad3",
			Undo:               "Numpad8",
			Skip:               "Numpad2",
			Pause:              "Numpad5",
			PreviousComparison: "Numpad4",
			NextComparison:     "Numpad6",
		},
	}
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "livesplit-overlay", "config.toml")
}

func Load(path string, mustExist bool) (Config, error) {
	cfg := Default()
	md, err := toml.DecodeFile(path, &cfg)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !mustExist:
		return cfg, nil
	case err != nil:
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("config %s: unknown setting %q", path, undecoded[0].String())
	}
	cfg.Splits = expandHome(cfg.Splits)
	cfg.Layout = expandHome(cfg.Layout)
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	switch {
	case c.Width < 1 || c.Height < 1:
		return fmt.Errorf("width and height must be positive, got %dx%d", c.Width, c.Height)
	case c.FPS < 1 || c.FPS > 240:
		return fmt.Errorf("fps must be between 1 and 240, got %d", c.FPS)
	case c.Margin < 0:
		return fmt.Errorf("margin must not be negative, got %d", c.Margin)
	case c.Opacity <= 0 || c.Opacity > 1:
		return fmt.Errorf("opacity must be greater than 0 and at most 1, got %v", c.Opacity)
	}
	_, err := ParseCorner(c.Corner)
	return err
}

var corners = []string{"top-left", "top-right", "bottom-left", "bottom-right"}

func ParseCorner(s string) (int, error) {
	for i, name := range corners {
		if s == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("corner must be one of %s, got %q", strings.Join(corners, ", "), s)
}

func (h Hotkeys) Bindings() []Binding {
	return []Binding{
		{"split", h.Split},
		{"reset", h.Reset},
		{"undo", h.Undo},
		{"skip", h.Skip},
		{"pause", h.Pause},
		{"undo_all_pauses", h.UndoAllPauses},
		{"previous_comparison", h.PreviousComparison},
		{"next_comparison", h.NextComparison},
		{"toggle_timing_method", h.ToggleTimingMethod},
	}
}

func (h Hotkeys) JSON() string {
	m := map[string]any{}
	for _, b := range h.Bindings() {
		if b.Key == "" {
			m[b.Action] = nil
		} else {
			m[b.Action] = b.Key
		}
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func expandHome(path string) string {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, rest)
}
