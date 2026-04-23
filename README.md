# turing-display-go

Go app for Windows that drives a **Turing Smart Screen 3.5"** or compatible USB display.

## Features

- Auto-detects the display COM port from Windows registry data
- Opens the serial connection at `115200 baud`, `8N1`
- Sends the device `HELLO` handshake and prints a readable response
- Renders live CPU, RAM, GPU, and VRAM usage on the display
- Uses embedded tray and app icons, so no icon file is needed next to the built exe

## Requirements

- Windows 10 or 11
- Go 1.21 or newer
- The display connected by USB

## Project Layout

- [`cmd/main.go`](cmd/main.go) - Windows entrypoint
- [`internal/app`](internal/app) - startup and orchestration
- [`internal/chart`](internal/chart) - chart rendering and config parsing
- [`internal/frame`](internal/frame) - screen buffer helpers
- [`internal/sampler`](internal/sampler) - metric cadence helpers
- [`internal/win`](internal/win) - Windows APIs, tray, serial, and bitmap transfer
- [`config/config.json`](config/config.json) - sample config copied beside the exe at build time

## Build

```powershell
go build -o dist/turing-display.exe ./cmd
Copy-Item config\config.json dist\config.json
```

To regenerate the tray and app icon resources from the SVG source, run `build/windows/update-icon.sh` from a Bash shell.

## Run

```powershell
.\dist\turing-display.exe
```

## Config

The app looks for `config.json` next to the executable at runtime.

The build copies:

- source: `config/config.json`
- runtime: `dist/config.json`

## How It Works

The app scans known VID/PID pairs, opens the matching COM port, performs the handshake, and then updates the display once per turn using cached samples so slower metrics stay aligned.

## Notes

- The tray icon is embedded in the binary.
- The project is Windows-only.
