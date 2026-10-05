package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const example = `# my overlay
splits = "/runs/a.lss"
opacity = 1.0   # fade everything
outputs = [
  "DP-1",
]

[hotkeys]
split = "Numpad1" # main key
reset = "Numpad3"
`

func editExample(t *testing.T, change func(path string) error) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(example), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := change(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetReplacesAndKeepsComments(t *testing.T) {
	got := editExample(t, func(p string) error { return Set(p, "opacity", "0.6", nil) })
	if !strings.Contains(got, "opacity = 0.6  # fade everything\n") {
		t.Errorf("opacity line not replaced with comment kept:\n%s", got)
	}
	if !strings.HasPrefix(got, "# my overlay\n") {
		t.Errorf("leading comment lost:\n%s", got)
	}
}

func TestSetMultiLineArray(t *testing.T) {
	got := editExample(t, func(p string) error { return Set(p, "outputs", "HDMI-A-1, DP-2", nil) })
	if !strings.Contains(got, "outputs = [\"HDMI-A-1\", \"DP-2\"]\n\n[hotkeys]") {
		t.Errorf("multi-line array not replaced:\n%s", got)
	}
	cfg, err := Load(writeTemp(t, got), true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Outputs, ",") != "HDMI-A-1,DP-2" {
		t.Errorf("outputs = %v", cfg.Outputs)
	}
}

func TestSetAddsMissingKeys(t *testing.T) {
	got := editExample(t, func(p string) error {
		if err := Set(p, "corner", "bottom-left", nil); err != nil {
			return err
		}
		return Set(p, "hotkeys.pause", "Ctrl + KeyP", nil)
	})
	top, hotkeys, _ := strings.Cut(got, "[hotkeys]")
	if !strings.Contains(top, "corner = \"bottom-left\"\n") {
		t.Errorf("corner not added to the top level:\n%s", got)
	}
	if !strings.HasSuffix(hotkeys, "pause = \"Ctrl + KeyP\"\n") {
		t.Errorf("pause not added to [hotkeys]:\n%s", got)
	}
	if !strings.Contains(hotkeys, "split = \"Numpad1\" # main key\n") {
		t.Errorf("other hotkeys changed:\n%s", got)
	}
}

func TestSetCreatesFileAndSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := Set(path, "hotkeys.split", "F1", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "[hotkeys]\nsplit = \"F1\"\n" {
		t.Errorf("got %q", data)
	}
}

func TestSetRejectsInvalidValues(t *testing.T) {
	path := writeTemp(t, example)
	for _, tt := range [][2]string{{"opacity", "2"}, {"opacity", "half"}, {"corner", "middle"}, {"nope", "1"}} {
		if err := Set(path, tt[0], tt[1], nil); err == nil {
			t.Errorf("set %s=%s: no error", tt[0], tt[1])
		}
	}
	if data, _ := os.ReadFile(path); string(data) != example {
		t.Errorf("file changed after rejected values:\n%s", data)
	}
}

func TestUnset(t *testing.T) {
	got := editExample(t, func(p string) error { return Unset(p, "hotkeys.split", nil) })
	if strings.Contains(got, "split =") || !strings.Contains(got, "reset = \"Numpad3\"") {
		t.Errorf("unset removed the wrong lines:\n%s", got)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
