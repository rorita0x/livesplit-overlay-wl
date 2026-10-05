# livesplit-wayland-overlay

Shows a LiveSplit timer as an always-on-top overlay on Wayland. The overlay
sits in a screen corner, mouse clicks go through it, and it is controlled with
hotkeys.

It works on compositors with the layer-shell protocol: KDE Plasma (KWin), Sway,
Hyprland, niri and others. GNOME is not supported.

Under the hood, [livesplit-core](https://github.com/LiveSplit/livesplit-core)
(the engine behind LiveSplit One) runs the timer and draws the layout, and
[go-overlay](https://github.com/rorita0x/go-overlay) puts the picture on screen.

## Installation

You need:

- Go and Rust (`cargo`)
- the development files for `wayland-client` and `pangocairo`
- [go-overlay](https://github.com/rorita0x/go-overlay) checked out **next to**
  this folder, i.e. as `../go-overlay`

```sh
git submodule update --init   # downloads livesplit-core
make                          # builds ./livesplit-overlay
```

The first build compiles livesplit-core and takes a minute or two.

## Quick start

Try it with the included sample splits:

```sh
./livesplit-overlay -splits testdata/sample.lss
```

In a second terminal:

```sh
./livesplit-overlay split    # starts the timer, then splits
./livesplit-overlay reset
```

Stop the overlay with Ctrl+C.

## Configuration

Copy the example config and point it at your splits file:

```sh
mkdir -p ~/.config/livesplit-overlay
cp config.example.toml ~/.config/livesplit-overlay/config.toml
```

All settings are optional except `splits`.

| Setting       | Default       | What it does |
|---------------|---------------|--------------|
| `splits`      | –             | Your splits file (`.lss`). `~` is allowed. |
| `layout`      | built-in      | Layout file from LiveSplit (`.lsl`) or LiveSplit One (`.ls1l`). |
| `width`       | `300`         | Overlay width in pixels. |
| `height`      | `500`         | Overlay height in pixels. |
| `corner`      | `"top-right"` | `"top-left"`, `"top-right"`, `"bottom-left"` or `"bottom-right"`. |
| `margin`      | `12`          | Distance to the screen edges in pixels. |
| `opacity`     | `1.0`         | Transparency of the whole overlay, text included: `1.0` is solid, `0.5` half see-through. Must be above 0. |
| `outputs`     | `[]`          | Which monitors to show it on, e.g. `["DP-1"]`. Empty means all. |
| `over_panels` | `false`       | `true` draws over panels/taskbars; `false` keeps it next to them. |
| `fps`         | `30`          | How often the overlay redraws per second. |
| `autosave`    | `true`        | Save the splits file after every reset and when the overlay quits. |
| `language`    | system        | Language for labels such as "Best Possible Time", e.g. `"de"`. |

`width`, `height` and `margin` are logical pixels, the same units as your
desktop scaling uses. On a scaled monitor the overlay is still rendered sharp.

**See-through background with solid text:** don't use `opacity` for that.
Instead, give the layout's background a transparent color in the layout
settings of LiveSplit or LiveSplit One.

**Saving:** whenever the splits file is saved, the previous version is kept as
`<splits file>.bak`. You can also save by hand with `livesplit-overlay save`.

**Changing settings while running:** edit the config, then run
`livesplit-overlay reload`. Only `splits`, `outputs` and `over_panels` need a
restart of the overlay.

Command-line flags override the config: `-config <file>`, `-splits <file>`,
`-layout <file>`.

## Hotkeys

There are two ways to control the timer. You can use both at the same time.

### 1. Built-in hotkeys

Set the keys in the `[hotkeys]` section of the config:

```toml
[hotkeys]
enabled = true
split = "Numpad1"
reset = "Numpad3"
undo = "Numpad8"
skip = "Numpad2"
pause = "Numpad5"
undo_all_pauses = ""
previous_comparison = "Numpad4"
next_comparison = "Numpad6"
toggle_timing_method = ""
```

- Key names follow the [web key code names](https://developer.mozilla.org/docs/Web/API/UI_Events/Keyboard_event_code_values):
  `KeyA`, `Digit1`, `F1`, `Space`, `Numpad1`, `ArrowUp`, …
- Combinations: `"Ctrl + KeyS"`, `"Shift + Alt + F1"` (modifiers: `Ctrl`,
  `Alt`, `Shift`, `Meta`).
- `""` leaves an action without a key.
- The key still reaches the game; it isn't swallowed.

**Required on Wayland:** these hotkeys read the keyboard directly, so your user
must be in the `input` group. Run this once, then log out and back in:

```sh
sudo usermod -aG input $USER
```

The overlay prints a warning at startup if this is missing.

### 2. Shortcuts from your desktop

Every action is also a command you can bind to a key in your desktop's
shortcut settings. This works without the `input` group.

| Command                             | Action |
|-------------------------------------|--------|
| `livesplit-overlay split`           | Start the timer, or split |
| `livesplit-overlay reset`           | Reset the attempt |
| `livesplit-overlay undo`            | Undo the last split |
| `livesplit-overlay skip`            | Skip the current split |
| `livesplit-overlay pause`           | Pause / resume |
| `livesplit-overlay undo-pauses`     | Remove all pause time from the attempt |
| `livesplit-overlay prev-comparison` | Show the previous comparison |
| `livesplit-overlay next-comparison` | Show the next comparison |
| `livesplit-overlay toggle-timing`   | Switch between Real Time and Game Time |
| `livesplit-overlay scroll-up`       | Scroll the splits list up |
| `livesplit-overlay scroll-down`     | Scroll the splits list down |
| `livesplit-overlay save`            | Save the splits file now |
| `livesplit-overlay reload`          | Re-read the config |

Where to bind them:

- **KDE Plasma:** System Settings → Keyboard → Shortcuts → Add New → Command
  or Script, then enter e.g. `/path/to/livesplit-overlay split`.
- **Hyprland:** `bind = , KP_End, exec, livesplit-overlay split`
- **Sway:** `bindsym KP_End exec livesplit-overlay split`

Use the full path to `livesplit-overlay` unless it is in your `PATH`.
