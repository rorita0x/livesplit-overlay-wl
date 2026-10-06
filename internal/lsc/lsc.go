package lsc

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/bindings
#cgo LDFLAGS: ${SRCDIR}/../../third_party/livesplit-core/target/release/liblivesplit_core.a
#cgo LDFLAGS: -ldl -lgcc_s -lutil
#cgo LDFLAGS: -lrt -lpthread -lm -lc
#include <stdlib.h>
#include "livesplit_core.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"slices"
	"strings"
	"unsafe"
)

type Lang uint8

func ParseLocale(locale string) Lang {
	locale, _, _ = strings.Cut(locale, ".")
	locale = strings.ReplaceAll(locale, "_", "-")
	cs := C.CString(locale)
	defer C.free(unsafe.Pointer(cs))
	return Lang(C.Lang_parse_locale(cs))
}

type Phase uint8

const (
	NotRunning Phase = iota
	Running
	Ended
	Paused
)

type Command string

const (
	Split          Command = "split"
	Reset          Command = "reset"
	Undo           Command = "undo"
	Skip           Command = "skip"
	Pause          Command = "pause"
	UndoPauses     Command = "undo-pauses"
	PrevComparison Command = "prev-comparison"
	NextComparison Command = "next-comparison"
	ToggleTiming   Command = "toggle-timing"
)

var Commands = []Command{Split, Reset, Undo, Skip, Pause, UndoPauses, PrevComparison, NextComparison, ToggleTiming}

type Timer struct {
	shared C.SharedTimer
}

func NewTimer(lss []byte, path string) (*Timer, error) {
	if len(lss) == 0 {
		return nil, errors.New("splits file is empty")
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	res := C.Run_parse(unsafe.Pointer(&lss[0]), C.size_t(len(lss)), cpath)
	if !C.ParseRunResult_parsed_successfully(res) {
		C.ParseRunResult_drop(res)
		return nil, errors.New("splits file could not be parsed")
	}
	timer := C.Timer_new(C.ParseRunResult_unwrap(res))
	if timer == nil {
		return nil, errors.New("splits file has no segments")
	}
	return &Timer{shared: C.Timer_into_shared(timer)}, nil
}

func (t *Timer) Close() {
	C.SharedTimer_drop(t.shared)
}

func (t *Timer) read(fn func(C.TimerRef)) {
	lock := C.SharedTimer_read(t.shared)
	defer C.TimerReadLock_drop(lock)
	fn(C.TimerReadLock_timer(lock))
}

func (t *Timer) write(fn func(C.TimerRefMut)) {
	lock := C.SharedTimer_write(t.shared)
	defer C.TimerWriteLock_drop(lock)
	fn(C.TimerWriteLock_timer(lock))
}

func (t *Timer) Do(cmd Command) error {
	if !slices.Contains(Commands, cmd) {
		return fmt.Errorf("unknown command %q", cmd)
	}
	var result C.int32_t
	t.write(func(tm C.TimerRefMut) {
		switch cmd {
		case Split:
			result = C.Timer_split_or_start(tm)
		case Reset:
			result = C.Timer_reset(tm, true)
		case Undo:
			result = C.Timer_undo_split(tm)
		case Skip:
			result = C.Timer_skip_split(tm)
		case Pause:
			result = C.Timer_toggle_pause_or_start(tm)
		case UndoPauses:
			result = C.Timer_undo_all_pauses(tm)
		case PrevComparison:
			C.Timer_switch_to_previous_comparison(tm)
		case NextComparison:
			C.Timer_switch_to_next_comparison(tm)
		case ToggleTiming:
			C.Timer_toggle_timing_method(tm)
		}
	})
	if result < 0 {
		return fmt.Errorf("%s is not possible right now", cmd)
	}
	return nil
}

func (t *Timer) Phase() Phase {
	var p Phase
	t.read(func(tm C.TimerRef) { p = Phase(C.Timer_current_phase(tm)) })
	return p
}

func (t *Timer) ModifiedLSS() (lss string, modified bool) {
	t.read(func(tm C.TimerRef) {
		if C.Run_has_been_modified(C.Timer_get_run(tm)) {
			lss, modified = C.GoString(C.Timer_save_as_lss(tm)), true
		}
	})
	return lss, modified
}

func (t *Timer) LSS() string {
	var lss string
	t.read(func(tm C.TimerRef) { lss = C.GoString(C.Timer_save_as_lss(tm)) })
	return lss
}

func (t *Timer) MarkSaved() {
	t.write(func(tm C.TimerRefMut) { C.Timer_mark_as_unmodified(tm) })
}

type Renderer struct {
	lang     Lang
	layout   C.Layout
	state    C.LayoutState
	cache    C.ImageCache
	renderer C.SoftwareRenderer
	frames   int
}

func NewRenderer(layoutPath string, lang Lang) (*Renderer, error) {
	var layout C.Layout
	if layoutPath == "" {
		layout = C.Layout_default_layout(C.uint8_t(lang))
	} else {
		data, err := os.ReadFile(layoutPath)
		if err != nil {
			return nil, err
		}
		layout = parseLayout(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
		if layout == nil {
			return nil, fmt.Errorf("%s: not a LiveSplit layout (.ls1l or .lsl)", layoutPath)
		}
	}
	return &Renderer{
		lang:     lang,
		layout:   layout,
		state:    C.LayoutState_new(),
		cache:    C.ImageCache_new(),
		renderer: C.SoftwareRenderer_new(),
	}, nil
}

func parseLayout(data []byte) C.Layout {
	if len(data) == 0 {
		return nil
	}
	cjson := C.CString(string(data))
	defer C.free(unsafe.Pointer(cjson))
	if layout := C.Layout_parse_json(cjson); layout != nil {
		return layout
	}
	return C.Layout_parse_original_livesplit(unsafe.Pointer(&data[0]), C.size_t(len(data)))
}

func (r *Renderer) Close() {
	C.SoftwareRenderer_drop(r.renderer)
	C.ImageCache_drop(r.cache)
	C.LayoutState_drop(r.state)
	C.Layout_drop(r.layout)
}

func (r *Renderer) ScrollUp()   { C.Layout_scroll_up(r.layout) }
func (r *Renderer) ScrollDown() { C.Layout_scroll_down(r.layout) }

func (r *Renderer) Render(t *Timer, img *image.RGBA, forceRedraw bool) {
	t.read(func(tm C.TimerRef) {
		C.Layout_update_state(r.layout, r.state, r.cache, tm, C.uint8_t(r.lang))
	})
	b := img.Bounds()
	C.SoftwareRenderer_render(r.renderer, r.state, r.cache, (*C.uint8_t)(unsafe.Pointer(&img.Pix[0])),
		C.uint32_t(b.Dx()), C.uint32_t(b.Dy()), C.uint32_t(img.Stride/4), C.bool(forceRedraw))

	if r.frames++; r.frames%600 == 0 {
		C.ImageCache_collect(r.cache)
	}
}

type Hotkeys struct {
	sink   C.CommandSink
	system C.HotkeySystem
}

func ValidHotkey(key string) bool {
	return parseHotkeyConfig(fmt.Sprintf(`{"split":%q}`, key)) != nil
}

func parseHotkeyConfig(json string) C.HotkeyConfig {
	cs := C.CString(json)
	defer C.free(unsafe.Pointer(cs))
	return C.HotkeyConfig_parse_json(cs)
}

func NewHotkeys(t *Timer, configJSON string) (*Hotkeys, error) {
	config := parseHotkeyConfig(configJSON)
	if config == nil {
		return nil, errors.New("invalid hotkey configuration")
	}
	sink := C.CommandSink_from_timer(C.SharedTimer_share(t.shared))
	system := C.HotkeySystem_with_config(sink, config)
	if system == nil {
		C.CommandSink_drop(sink)
		return nil, errors.New("global hotkeys are not available (on Wayland your user must be in the 'input' group)")
	}
	return &Hotkeys{sink: sink, system: system}, nil
}

func (h *Hotkeys) Close() {
	C.HotkeySystem_drop(h.system)
	C.CommandSink_drop(h.sink)
}
