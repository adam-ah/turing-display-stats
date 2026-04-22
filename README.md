# turing-display-go

Communicate with a **Turing Smart Screen 3.5"** (or compatible) display from Go on Windows.

## What it does

1. **Auto-detects** the display's COM port by scanning the Windows registry for known VID/PID pairs
2. Opens the serial connection at **115200 baud, 8N1**
3. Sends the protocol **HELLO handshake** (6 × `0x45`) and reads the device's response
4. Prints a human-readable interpretation of what the display reports back
5. Renders live **GPU VRAM**, **GPU 3D load**, **RAM usage**, and **CPU usage** on the display

## Requirements

- **Windows** 10/11 (uses Windows Registry + CreateFile API)
- **Go 1.21+** installed
- The Turing display connected via USB

## Build & run

```powershell
cd go_display
go mod tidy
go build -o turing-display.exe
.\turing-display.exe
```

## How it works

### Device detection

The tool reads `HKEY_LOCAL_MACHINE\HARDWARE\DEVICEMAP\SERIALCOMM` to enumerate all COM ports, then walks the USB registry tree (`HKLM\System\CurrentControlSet\Enum\USB\`) to match each port against known device signatures:

| VID    | PID    | Device                        |
|--------|--------|-------------------------------|
| 1A86   | 5722   | Turing Smart Screen / UsbPCMonitor (Rev A/B) |
| 454D   | 4E41   | Kipye Qiye Smart Display (Rev D)    |

### Serial protocol

The Turing 3.5" uses a simple UART protocol:
- **Baud rate:** 115200
- **No flow control** required for basic operations
- **HELLO command:** Send 6 bytes of `0x45`, read 6 bytes back to identify the device variant

### Adding more functionality

To display an image, you would:
1. Convert your image to **RGB565** (16-bit per pixel)
2. Send a `DISPLAY_BITMAP` command with coordinates
3. Stream the raw RGB565 pixel data in chunks

See the Python library's `library/lcd/serialize.py` for the exact RGB565 conversion formula:
```
R5 << 11 | G6 << 5 | B5   (little-endian, 2 bytes per pixel)
```

## License

Same as the parent project: GPL-3.0-or-later
