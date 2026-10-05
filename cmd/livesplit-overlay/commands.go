package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	overlay "github.com/rorita0x/go-overlay"

	"livesplit-wayland-overlay/internal/config"
	"livesplit-wayland-overlay/internal/control"
)

var restartSettings = []string{"splits", "outputs", "over_panels"}

func listOutputs() error {
	list, err := overlay.ListOutputs()
	if err != nil {
		return err
	}
	cfg, _ := config.Load(config.DefaultPath(), false)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCALE\tMONITOR\t")
	for _, out := range list {
		mark := ""
		if slices.Contains(cfg.Outputs, out.Name) {
			mark = "(in use)"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", out.Name, out.Scale, out.Description, mark)
	}
	w.Flush()
	if len(cfg.Outputs) == 0 && len(list) > 0 {
		fmt.Println("\nThe overlay shows on all of them. To pick one: livesplit-overlay config set outputs " + list[0].Name)
	}
	return nil
}

const configUsage = `Usage:
  livesplit-overlay config                       show all settings
  livesplit-overlay config get <setting>         show one setting
  livesplit-overlay config set <setting> <value> change a setting
  livesplit-overlay config unset <setting>       go back to the default
  livesplit-overlay config path                  print the config file location

Add -config <file> before the subcommand to use another file.

Examples:
  livesplit-overlay config set splits ~/splits/game.lss
  livesplit-overlay config set opacity 0.8
  livesplit-overlay config set outputs DP-1            (several: DP-1,HDMI-A-1)
  livesplit-overlay config set hotkeys.split "Ctrl + KeyS"
  livesplit-overlay config set hotkeys.undo ""         (no key)

A running overlay picks up the change right away.
`

func configCommand(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), configUsage) }
	path := fs.String("config", config.DefaultPath(), "config file")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	args = fs.Args()
	sub := "show"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}

	wantArgs := map[string]int{"show": 0, "path": 0, "get": 1, "unset": 1, "set": 2, "help": 0}
	n, known := wantArgs[sub]
	if !known || len(args) != n {
		fmt.Fprint(os.Stderr, configUsage)
		if !known {
			return fmt.Errorf("unknown config subcommand %q", sub)
		}
		return fmt.Errorf("config %s takes %d argument(s)", sub, n)
	}

	switch sub {
	case "help":
		fmt.Print(configUsage)
		return nil
	case "path":
		fmt.Println(*path)
		return nil
	case "show", "get":
		cfg, err := config.Load(*path, *path != config.DefaultPath())
		if err != nil {
			return err
		}
		if sub == "get" {
			s, err := cfg.Setting(args[0])
			if err != nil {
				return err
			}
			printSetting(s)
			return nil
		}
		return showConfig(cfg, *path)
	case "set":
		if err := config.Set(*path, args[0], args[1], validateHotkeys); err != nil {
			return err
		}
	case "unset":
		if err := config.Unset(*path, args[0], validateHotkeys); err != nil {
			return err
		}
	}
	return applyChange(args[0], *path)
}

func showConfig(cfg config.Config, path string) error {
	if _, err := os.Stat(path); err != nil {
		fmt.Printf("# %s does not exist yet, showing defaults\n", path)
	} else {
		fmt.Printf("# %s\n", path)
	}
	section := ""
	for _, s := range cfg.Settings() {
		if sec, _, ok := strings.Cut(s.Name, "."); ok && sec != section {
			section = sec
			fmt.Printf("\n[%s]\n", section)
		}
		fmt.Println(s.TOML())
	}
	return nil
}

func printSetting(s config.Setting) {
	_, value, _ := strings.Cut(s.TOML(), " = ")
	fmt.Printf("%s = %s\n", s.Name, value)
}

func applyChange(setting, path string) error {
	cfg, _ := config.Load(path, true)
	if s, err := cfg.Setting(setting); err == nil {
		printSetting(s)
	}
	if path != config.DefaultPath() {
		return nil
	}
	if err := control.Send(control.SocketPath(), cmdReload); err != nil {
		return nil
	}
	if slices.Contains(restartSettings, setting) {
		fmt.Println("Saved. Restart the overlay for this setting to take effect.")
	} else {
		fmt.Println("Applied to the running overlay.")
	}
	return nil
}
