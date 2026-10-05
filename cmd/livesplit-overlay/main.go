package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"os/signal"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	overlay "github.com/rorita0x/go-overlay"

	"livesplit-wayland-overlay/internal/config"
	"livesplit-wayland-overlay/internal/control"
	"livesplit-wayland-overlay/internal/lsc"
)

const (
	cmdSave       = "save"
	cmdReload     = "reload"
	cmdScrollUp   = "scroll-up"
	cmdScrollDown = "scroll-down"
)

func clientCommands() []string {
	cmds := []string{cmdSave, cmdReload, cmdScrollUp, cmdScrollDown}
	for _, c := range lsc.Commands {
		cmds = append(cmds, string(c))
	}
	return cmds
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("livesplit-overlay: ")

	if len(os.Args) > 1 && slices.Contains(clientCommands(), os.Args[1]) {
		if err := control.Send(control.SocketPath(), os.Args[1]); err != nil {
			log.Fatal(err)
		}
		return
	}

	configPath := flag.String("config", "", "config file (default "+config.DefaultPath()+")")
	splits := flag.String("splits", "", "splits file (.lss), overrides the config")
	layout := flag.String("layout", "", "layout file (.ls1l or .lsl), overrides the config")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage:\n  %[1]s [flags]       start the overlay\n  %[1]s <command>     send a command to the running overlay\n\nCommands:\n  %s\n\nFlags:\n",
			os.Args[0], strings.Join(clientCommands(), " "))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		log.Printf("unknown command %q, see -h", flag.Arg(0))
		os.Exit(2)
	}

	a := &app{configPath: *configPath, splitsFlag: *splits, layoutFlag: *layout}
	if err := a.run(); err != nil {
		log.Fatal(err)
	}
}

type request struct {
	cmd   string
	reply chan error
}

type app struct {
	configPath, splitsFlag, layoutFlag string

	cfg      config.Config
	corner   overlay.Corner
	timer    *lsc.Timer
	ov       *overlay.Overlay
	renderer *lsc.Renderer
	hotkeys  *lsc.Hotkeys
	requests chan request
}

func (a *app) loadConfig() (config.Config, error) {
	path, mustExist := a.configPath, true
	if path == "" {
		path, mustExist = config.DefaultPath(), false
	}
	cfg, err := config.Load(path, mustExist)
	if err != nil {
		return cfg, err
	}
	if a.splitsFlag != "" {
		cfg.Splits = a.splitsFlag
	}
	if a.layoutFlag != "" {
		cfg.Layout = a.layoutFlag
	}
	if cfg.Splits == "" {
		return cfg, fmt.Errorf("no splits file: set splits in %s or pass -splits", path)
	}
	for _, b := range cfg.Hotkeys.Bindings() {
		if b.Key != "" && !lsc.ValidHotkey(b.Key) {
			return cfg, fmt.Errorf("hotkeys.%s: invalid key %q (use names like \"Numpad1\", \"KeyS\", \"F1\" or \"Ctrl + KeyS\")", b.Action, b.Key)
		}
	}
	return cfg, nil
}

func (a *app) lang() lsc.Lang {
	if a.cfg.Language != "" {
		return lsc.ParseLocale(a.cfg.Language)
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			return lsc.ParseLocale(v)
		}
	}
	return lsc.ParseLocale("en")
}

func (a *app) run() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	a.cfg = cfg
	c, _ := config.ParseCorner(cfg.Corner)
	a.corner = overlay.Corner(c)

	lss, err := os.ReadFile(cfg.Splits)
	if err != nil {
		return err
	}
	if a.timer, err = lsc.NewTimer(lss, cfg.Splits); err != nil {
		return fmt.Errorf("%s: %w", cfg.Splits, err)
	}
	defer a.timer.Close()

	if a.renderer, err = lsc.NewRenderer(cfg.Layout, a.lang()); err != nil {
		return err
	}
	defer func() { a.renderer.Close() }()

	a.startHotkeys()
	defer a.stopHotkeys()

	opts := overlay.Options{Namespace: "livesplit-overlay", Outputs: cfg.Outputs}
	if cfg.OverPanels {
		opts.ExclusiveZone = -1
	}
	if a.ov, err = overlay.New(opts); err != nil {
		return err
	}
	defer a.ov.Close()

	ln, err := control.Listen(control.SocketPath())
	if err != nil {
		return err
	}
	defer os.Remove(control.SocketPath())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.requests = make(chan request)
	go control.Serve(ctx, ln, a.submit(ctx))

	overlayErr := make(chan error, 1)
	go func() {
		overlayErr <- a.ov.Run(ctx)
		stop()
	}()

	a.loop(ctx)
	if a.cfg.Autosave {
		a.saveIfModified()
	}
	return <-overlayErr
}

func (a *app) submit(ctx context.Context) func(cmd string) error {
	return func(cmd string) error {
		req := request{cmd: cmd, reply: make(chan error, 1)}
		select {
		case a.requests <- req:
			return <-req.reply
		case <-ctx.Done():
			return errors.New("shutting down")
		}
	}
}

func (a *app) startHotkeys() {
	if !a.cfg.Hotkeys.Enabled {
		return
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" && !inInputGroup() {
		log.Print("warning: global hotkeys on Wayland need your user in the 'input' group " +
			"(sudo usermod -aG input $USER, then log in again); use compositor shortcuts running `livesplit-overlay split` etc. meanwhile")
	}
	h, err := lsc.NewHotkeys(a.timer, a.cfg.Hotkeys.JSON())
	if err != nil {
		log.Printf("warning: %v", err)
		return
	}
	a.hotkeys = h
}

func (a *app) stopHotkeys() {
	if a.hotkeys != nil {
		a.hotkeys.Close()
		a.hotkeys = nil
	}
}

func inInputGroup() bool {
	g, err := user.LookupGroup("input")
	if err != nil {
		return false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return false
	}
	groups, _ := os.Getgroups()
	return slices.Contains(groups, gid)
}

func (a *app) loop(ctx context.Context) {
	tick := time.NewTicker(time.Second / time.Duration(a.cfg.FPS))
	defer tick.Stop()

	var img *image.RGBA
	var shown []byte
	lastPhase := a.timer.Phase()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-a.requests:
			fps := a.cfg.FPS
			err := a.handle(req.cmd)
			req.reply <- err
			if req.cmd == cmdReload && err == nil {
				img = nil
				if a.cfg.FPS != fps {
					tick.Reset(time.Second / time.Duration(a.cfg.FPS))
				}
			}
		case <-tick.C:
		}

		sc := a.ov.Scale()
		w, h := a.cfg.Width*sc, a.cfg.Height*sc
		redraw := img == nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h
		if redraw {
			img = image.NewRGBA(image.Rect(0, 0, w, h))
			shown = nil
		}
		a.renderer.Render(a.timer, img, redraw)
		if !bytes.Equal(img.Pix, shown) {
			a.ov.SetImage(a.corner, overlay.Image{RGBA: img, Scale: sc, Margin: a.cfg.Margin, Opacity: a.cfg.Opacity})
			shown = append(shown[:0], img.Pix...)
		}

		phase := a.timer.Phase()
		if a.cfg.Autosave && phase == lsc.NotRunning && lastPhase != lsc.NotRunning {
			a.saveIfModified()
		}
		lastPhase = phase
	}
}

func (a *app) handle(cmd string) error {
	switch cmd {
	case cmdSave:
		return a.save(a.timer.LSS())
	case cmdReload:
		return a.reload()
	case cmdScrollUp:
		a.renderer.ScrollUp()
		return nil
	case cmdScrollDown:
		a.renderer.ScrollDown()
		return nil
	}
	return a.timer.Do(lsc.Command(cmd))
}

func (a *app) reload() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	c, _ := config.ParseCorner(cfg.Corner)
	old := a.cfg
	if cfg.Splits != old.Splits || !slices.Equal(cfg.Outputs, old.Outputs) || cfg.OverPanels != old.OverPanels {
		log.Print("reload: splits, outputs and over_panels take effect after a restart")
	}
	cfg.Splits, cfg.Outputs, cfg.OverPanels = old.Splits, old.Outputs, old.OverPanels
	a.cfg = cfg
	renderer, err := lsc.NewRenderer(cfg.Layout, a.lang())
	if err != nil {
		a.cfg = old
		return err
	}
	a.renderer.Close()
	a.renderer = renderer

	if corner := overlay.Corner(c); corner != a.corner {
		a.ov.Clear(a.corner)
		a.corner = corner
	}
	a.stopHotkeys()
	a.startHotkeys()
	log.Print("configuration reloaded")
	return nil
}

func (a *app) saveIfModified() {
	lss, modified := a.timer.ModifiedLSS()
	if !modified {
		return
	}
	if err := a.save(lss); err != nil {
		log.Printf("autosave: %v", err)
	}
}

func (a *app) save(lss string) error {
	path := a.cfg.Splits
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", old, info.Mode().Perm()); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(lss), info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	a.timer.MarkSaved()
	log.Printf("saved splits to %s", path)
	return nil
}
