# livesplit-wayland-overlay

LiveSplit on Wayland: [livesplit-core](https://github.com/LiveSplit/livesplit-core)
renders the timer with its software renderer, and
[go-overlay](https://github.com/rorita0x/go-overlay) shows it as a
click-through layer-shell overlay (KWin, Sway, Hyprland, niri, …).

## Build

Needs Go, Rust (cargo), and the development files for `wayland-client` and
`pangocairo`. go-overlay is expected next to this repo (`../go-overlay`, see
the `replace` in `go.mod`).

```sh
git submodule update --init
make
```

## Run

```sh
mkdir -p ~/.config/livesplit-overlay
cp config.example.toml ~/.config/livesplit-overlay/config.toml   # set splits = …
./livesplit-overlay
./livesplit-overlay -splits testdata/sample.lss                  # quick try
```

Layouts from LiveSplit (`.lsl`) and LiveSplit One (`.ls1l`) both work; without
one the default layout is used. The image is rendered at `width`×`height`
logical pixels, at the output's buffer scale.

With `autosave = true` the splits file is written after every reset and on exit
(the previous version is kept as `<file>.bak`). `livesplit-overlay save` saves
on demand.

## Hotkeys

Two ways, usable at the same time:

1. **Built-in global hotkeys** from `[hotkeys]` in the config (LiveSplit key
   names such as `Numpad1`, `KeyS`, `F1`, `Ctrl + KeyS`). On Wayland these read
   the keyboard through evdev, so your user must be in the `input` group:

   ```sh
   sudo usermod -aG input $USER   # then log out and back in
   ```

2. **Compositor shortcuts** that run a command against the running overlay:

   ```sh
   livesplit-overlay split | reset | undo | skip | pause | undo-pauses
   livesplit-overlay prev-comparison | next-comparison | toggle-timing
   livesplit-overlay scroll-up | scroll-down | save | reload
   ```

   KDE: System Settings → Shortcuts → Add New → Command.
   Hyprland: `bind = , KP_End, exec, livesplit-overlay split`.
   Sway: `bindsym KP_End exec livesplit-overlay split`.

`livesplit-overlay reload` re-reads the config (layout, size, corner, margin,
fps, language, hotkeys). Changes to `splits`, `outputs` and `over_panels` need a
restart.
