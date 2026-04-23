//go:build windows

// turing-display-go: Communicate with Turing Smart Screen displays from Go.
// Build: go build -ldflags="-H windowsgui" -o turing-display.exe
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ---------------------------------------------------------------------------
// Debug logger — only writes when --debug is set
// ---------------------------------------------------------------------------

var debugEnabled bool

func init() {
	flag.BoolVar(&debugEnabled, "debug", false, "enable console output")
}

type debugLogger struct {
	mu sync.Mutex
}

func (l *debugLogger) Printf(format string, v ...interface{}) {
	if !debugEnabled {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	log.Printf(format, v...)
}

var dbg debugLogger

// ---------------------------------------------------------------------------
// Screen constants
// ---------------------------------------------------------------------------

const (
	screenWidth  = 320
	screenHeight = 480
	divColor     = 0x80 // gray divider color (RGB 128,128,128)
)

// Layout constants per section (120px each).
const (
	sectionHeight   = 120
	labelZoneHeight = 15 // rows 1..15 after divider (textH=13 + 2px padding)
	chartOffset     = 16 // chart starts at sectionStart + 16
)

// sectionStart returns the top row of each section.
var sectionStart = [4]int{0, 120, 240, 360}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	flag.Parse()
	log.SetFlags(0)

	dbg.Printf("=== Turing Smart Screen ===")

	// Init tray icon
	if err := initTray(); err != nil {
		dbg.Printf("tray init warning: %v", err)
	} else {
		defer removeTray()
	}

	// 1. Find the device
	dbg.Printf("Scanning for Turing display...")
	comPort, devName, err := findTuringDisplay()
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	dbg.Printf("Found: %s on %s", devName, comPort)

	// 2. Open serial connection
	dbg.Printf("Opening serial port at 115200 baud...")
	handle, err := openSerial(comPort)
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	defer closeSerial(handle)
	dbg.Printf("Serial port opened.")

	// 3. Send HELLO handshake
	dbg.Printf("Sending HELLO handshake...")
	resp, err := sendHello(handle)
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	dbg.Printf("Response (%d bytes): %s", len(resp), interpretHello(resp))

	// 4. Load config
	chartCfg, err := loadConfig("config.json")
	if err != nil {
		dbg.Printf("Warning: no config.json: %v", err)
		chartCfg = nil
	}

	// 5. Init GPU/CPU samplers
	statsQuery, err := newGpuPdhQuery()
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	defer statsQuery.close()

	cpuSampler, err := newCpuUsageSampler()
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}

	// 6. Get first stats snapshot (needed for RAM/VRAM labels)
	stats, err := statsQuery.snapshot()
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	memory, err := readSystemMemoryStats()
	if err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}

	// 7. Init charts
	var charts [4]*Chart // cpu, ram, gpu, vram
	if chartCfg != nil {
		names := [4]string{"cpu", "ram", "gpu", "vram"}
		for i, name := range names {
			if gCfg, ok := chartCfg.Graphs[name]; ok {
				charts[i] = newChart(gCfg, chartCfg.DotSize, chartCfg.Thresholds, true) // noLines=true
				dbg.Printf("%s chart: %dx%d at (%d,%d), capacity=%d",
					name, gCfg.Width, gCfg.Height, gCfg.X, gCfg.Y, charts[i].capacity)
			}
		}
	}

	// 8. Draw base frame (white + dividers + labels)
	labels := [4]string{
		"CPU",
		fmt.Sprintf("RAM - %s", formatBytesGiB(memory.totalBytes)),
		"GPU",
		fmt.Sprintf("VRAM - %s", formatBytesGiB(stats.totalBytes)),
	}
	base := renderBaseFrame(labels)
	if err := sendDisplayBitmapRevA(handle, 0, 0, screenWidth-1, screenHeight-1, base); err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	dbg.Printf("Base frame sent.")

	// 9. Signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	// 10. Update function
	updateStats := func() error {
		stats, err := statsQuery.snapshot()
		if err != nil {
			return err
		}
		memory, err := readSystemMemoryStats()
		if err != nil {
			return err
		}
		cpuPct, err := cpuSampler.snapshot()
		if err != nil {
			return err
		}

		ramPct := memory.loadPct
		gpuPct := stats.utilPct
		var vramPct float64
		if stats.totalBytes > 0 {
			vramPct = float64(stats.usedBytes) * 100 / float64(stats.totalBytes)
		}

		// Update each chart and send dirty regions.
		for i, ch := range charts {
			if ch == nil {
				continue
			}
			var value float64
			switch i {
			case 0:
				value = cpuPct
			case 1:
				value = ramPct
			case 2:
				value = gpuPct
			case 3:
				value = vramPct
			}
			for _, r := range ch.update(value) {
				if err := sendDisplayBitmapRevA(handle, r.X, r.Y,
					r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
					return err
				}
			}
		}

		dbg.Printf("Refreshed: CPU=%.0f%% RAM=%.0f%% GPU=%.0f%% VRAM=%.0f%%",
			cpuPct, ramPct, gpuPct, vramPct)
		return nil
	}

	// Initial update
	if err := updateStats(); err != nil {
		dbg.Printf("Error: %v", err)
		os.Exit(1)
	}
	dbg.Printf("Running... (Ctrl+C to stop)")

	// 11. Main loop
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-interrupt:
			dbg.Printf("Stopped.")
			return
		case <-exitApp:
			dbg.Printf("Exit via tray menu.")
			return
		case <-ticker.C:
			if err := updateStats(); err != nil {
				dbg.Printf("Error: %v", err)
				os.Exit(1)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Base frame — white canvas with 4 dividers and labels
//
// Each section (120px):
//   row 0:        gray divider line
//   rows 1..13:   label zone (white bg, centered text)
//   rows 14..119: chart area (106px)
// ---------------------------------------------------------------------------

func renderBaseFrame(labels [4]string) *image.RGBA {
	white := color.RGBA{255, 255, 255, 255}
	gray := color.RGBA{divColor, divColor, divColor, 255}

	img := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	metrics := face.Metrics()

	for i, label := range labels {
		start := sectionStart[i]

		// Gray divider line at top of section.
		for x := 0; x < screenWidth; x++ {
			img.Set(x, start, gray)
		}

		// Label zone: rows start+1 .. start+labelZoneHeight
		labelTop := start + 1

		// Measure text.
		textWidth := font.MeasureString(face, label).Ceil()
		padding := 4
		rectW := textWidth + padding
		rectX := (screenWidth - rectW) / 2

		// Center text vertically in label zone using correct baseline math.
		// Text occupies: baseline-ascent .. baseline+descent (total = ascent+descent).
		textH := metrics.Ascent.Ceil() + metrics.Descent.Ceil() // 13
		textTop := labelTop + (labelZoneHeight-textH)/2         // 1+(15-13)/2 = 2
		baseline := textTop + metrics.Ascent.Ceil()             // 2+11 = 13
		textBot := baseline + metrics.Descent.Ceil()            // 13+2 = 15

		// White background covering the full text extent.
		draw.Draw(img, image.Rect(rectX, textTop, rectX+rectW, textBot+1),
			&image.Uniform{white}, image.Point{}, draw.Src)

		// Draw label text.
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.RGBA{0, 0, 0, 255}),
			Face: face,
		}
		d.Dot = fixed.P(rectX+padding/2, baseline)
		d.DrawString(label)
	}

	return img
}
