//go:build windows

// turing-display-go: Communicate with Turing Smart Screen displays from Go.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func renderTextImage(width, height int, text string) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(color.Black),
		Face: face,
	}
	textWidth := d.MeasureString(text).Ceil()
	metrics := face.Metrics()
	x := (width - textWidth) / 2
	y := (height + metrics.Ascent.Ceil() - metrics.Descent.Ceil()) / 2
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
	return canvas
}

func renderTextBox(width, height int, text string) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	face := basicfont.Face7x13
	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(color.Black),
		Face: face,
	}
	metrics := face.Metrics()
	textWidth := d.MeasureString(text).Ceil()
	x := (width - textWidth) / 2
	y := (height + metrics.Ascent.Ceil() - metrics.Descent.Ceil()) / 2
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
	return canvas
}

func main() {
	log.SetFlags(0)

	fmt.Println("=== Turing Smart Screen — Go Hello ===")
	fmt.Println()

	// 1. Find the device
	fmt.Println("[1/3] Scanning for Turing display...")
	comPort, devName, err := findTuringDisplay()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("  Found: %s\n", devName)
	fmt.Printf("  COM port: %s\n", comPort)
	fmt.Println()

	// 2. Open serial connection
	fmt.Println("[2/3] Opening serial port at 115200 baud...")
	handle, err := openSerial(comPort)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer closeSerial(handle)
	fmt.Println("  Serial port opened successfully.")
	fmt.Println()

	// 3. Send HELLO and read response
	fmt.Println("[3/3] Sending HELLO handshake (6 × 0x45)...")
	resp, err := sendHello(handle)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("  Response (%d bytes): ", len(resp))
	for _, b := range resp {
		fmt.Printf("%02X ", b)
	}
	fmt.Println()
	fmt.Println()
	fmt.Printf("  → %s\n", interpretHello(resp))
	fmt.Println()
	fmt.Println("Done!")

	// 4. Render hello + live stats
	fmt.Println()
	fmt.Println("[4/4] Rendering hello + live GPU/RAM/CPU stats...")

	// Load chart config
	chartCfg, err := loadConfig("config.json")
	if err != nil {
		log.Printf("Warning: could not load config.json: %v — charts disabled", err)
		chartCfg = nil
	}

	statsQuery, err := newGpuPdhQuery()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer statsQuery.close()

	cpuSampler, err := newCpuUsageSampler()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Initialise charts from config
	var cpuChart *Chart
	if chartCfg != nil {
		if gCfg, ok := chartCfg.Graphs["cpu"]; ok {
			cpuChart = newChart(gCfg, chartCfg.DotSize, chartCfg.Thresholds)
			fmt.Printf("  CPU chart: %dx%d at (%d,%d), capacity=%d dots\n",
				gCfg.Width, gCfg.Height, gCfg.X, gCfg.Y, cpuChart.capacity)
		}
	}

	// Draw the base frame
	base := renderTextImage(320, 480, "")
	draw.Draw(base, image.Rect(20, 36, 20+60, 36+16), &image.Uniform{color.White}, image.Point{}, draw.Src)
	helloDrawer := &font.Drawer{
		Dst:  base,
		Src:  image.NewUniform(color.Black),
		Face: basicfont.Face7x13,
	}
	helloDrawer.Dot = fixed.P(20, 48)
	helloDrawer.DrawString("hello")
	if err := sendDisplayBitmapRevA(handle, 0, 0, 319, 479, base); err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Println("  Sent the base frame.")

	// Text label positions
	statsX, statsY := 20, 80
	statsW, statsH := 180, 16
	utilX, utilY := 20, 98
	utilW, utilH := 100, 16
	ramX, ramY := 20, 128
	ramW, ramH := 220, 16
	cpuX, cpuY := 20, 146
	cpuW, cpuH := 100, 16

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

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

		line1 := fmt.Sprintf("VRAM %s / %s", formatBytesGiB(stats.usedBytes), formatBytesGiB(stats.totalBytes))
		line2 := fmt.Sprintf("3D %.0f%%", stats.utilPct)
		line3 := fmt.Sprintf("RAM %.0f%% %s / %s", memory.loadPct, formatBytesGiB(memory.usedBytes), formatBytesGiB(memory.totalBytes))
		line4 := fmt.Sprintf("CPU %.0f%%", cpuPct)

		if err := sendDisplayBitmapRevA(handle, statsX, statsY, statsX+statsW-1, statsY+statsH-1, renderTextBox(statsW, statsH, line1)); err != nil {
			return err
		}
		if err := sendDisplayBitmapRevA(handle, utilX, utilY, utilX+utilW-1, utilY+utilH-1, renderTextBox(utilW, utilH, line2)); err != nil {
			return err
		}
		if err := sendDisplayBitmapRevA(handle, ramX, ramY, ramX+ramW-1, ramY+ramH-1, renderTextBox(ramW, ramH, line3)); err != nil {
			return err
		}
		if err := sendDisplayBitmapRevA(handle, cpuX, cpuY, cpuX+cpuW-1, cpuY+cpuH-1, renderTextBox(cpuW, cpuH, line4)); err != nil {
			return err
		}

		// Update chart overlays
		if cpuChart != nil {
			for _, r := range cpuChart.update(cpuPct) {
				if err := sendDisplayBitmapRevA(handle, r.X, r.Y,
					r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := updateStats(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	for {
		select {
		case <-interrupt:
			fmt.Println("Stopped.")
			return
		case <-ticker.C:
			if err := updateStats(); err != nil {
				log.Fatalf("Error: %v", err)
			}
			fmt.Printf("  GPU VRAM + 3D, RAM + CPU refreshed\r")
		}
	}
}
