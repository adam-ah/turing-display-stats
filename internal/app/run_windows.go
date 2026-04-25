//go:build windows

// turing-display-go: Communicate with Turing Smart Screen displays from Go.
// Build: go build -ldflags="-H windowsgui" -o turing-display.exe
package app

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"turing-display-go/internal/chart"
	"turing-display-go/internal/frame"
	"turing-display-go/internal/sampler"
	"turing-display-go/internal/win"
)

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

func fatalApp(err error, recovered any) {
	win.ShowErrorDialog(fatalFailureTitle, formatFatalFailure(err, recovered))
	os.Exit(1)
}

func shouldStop(interrupt <-chan os.Signal, exit <-chan struct{}) bool {
	select {
	case <-interrupt:
		return true
	case <-exit:
		return true
	default:
		return false
	}
}

func Run() {
	defer func() {
		if r := recover(); r != nil {
			fatalApp(nil, r)
		}
	}()

	flag.Parse()
	log.SetFlags(0)

	dbg.Printf("=== Turing Smart Screen ===")

	if title, message, ok := debugStartupDialog(); ok {
		win.ShowErrorDialog(title, message)
	}

	if err := win.InitTray(); err != nil {
		dbg.Printf("tray init warning: %v", err)
	} else {
		defer win.RemoveTray()
	}

	dbg.Printf("Scanning for Turing display...")
	comPort, devName, err := win.FindTuringDisplay()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	dbg.Printf("Found: %s on %s", devName, comPort)

	dbg.Printf("Opening serial port at 115200 baud...")
	handle, err := win.OpenSerial(comPort)
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	defer win.CloseSerial(handle)
	dbg.Printf("Serial port opened.")

	dbg.Printf("Sending HELLO handshake...")
	resp, err := win.SendHello(handle)
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	dbg.Printf("Response (%d bytes): %s", len(resp), win.InterpretHello(resp))

	configPath := appConfigPath()
	chartCfg, err := chart.LoadConfig(configPath)
	if err != nil {
		dbg.Printf("Warning: no %s: %v", configPath, err)
		chartCfg = nil
	}

	statsQuery, err := win.NewGpuPdhQuery()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	defer statsQuery.Close()

	cpuSampler, err := win.NewCpuUsageSampler()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}

	stats, err := statsQuery.Snapshot()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	memory, err := win.ReadSystemMemoryStats()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}

	var charts [4]*chart.Chart
	var metricCaches [4]sampler.MetricCache
	for i := range metricCaches {
		metricCaches[i] = sampler.NewMetricCache(1)
	}
	if chartCfg != nil {
		names := [4]string{"cpu", "ram", "gpu", "vram"}
		for i, name := range names {
			if gCfg, ok := chartCfg.Graphs[name]; ok {
				charts[i] = chart.NewChart(gCfg, chartCfg.DotSize, chartCfg.Thresholds, chartCfg.Colors, chartCfg.Colors.Background.RGBA, gCfg.RefreshSec, true)
				metricCaches[i] = sampler.NewMetricCache(gCfg.RefreshSec)
				dbg.Printf("%s chart: %dx%d at (%d,%d), capacity=%d",
					name, gCfg.Width, gCfg.Height, gCfg.X, gCfg.Y, charts[i].Capacity())
			}
		}
	}

	labels := [4]string{
		"CPU",
		fmt.Sprintf("RAM - %s", win.FormatBytesGiB(memory.TotalBytes)),
		"GPU",
		fmt.Sprintf("VRAM - %s", win.FormatBytesGiB(stats.TotalBytes)),
	}
	fontColor := color.RGBA{0, 0, 0, 255}
	bgColor := color.RGBA{255, 255, 255, 255}
	if chartCfg != nil {
		fontColor = chartCfg.Colors.FontColor.RGBA
		bgColor = chartCfg.Colors.Background.RGBA
	}
	base := renderBaseFrame(labels, fontColor, bgColor)
	if err := win.SendDisplayBitmapRevA(handle, 0, 0, screenWidth-1, screenHeight-1, base); err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}
	dbg.Printf("Base frame sent.")
	screenFrame := frame.NewScreenFrame(base)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	updateStats := func(turn int) error {
		stats, err := statsQuery.Snapshot()
		if err != nil {
			return err
		}
		memory, err := win.ReadSystemMemoryStats()
		if err != nil {
			return err
		}
		cpuPct, err := cpuSampler.Snapshot()
		if err != nil {
			return err
		}

		ramPct := memory.LoadPct
		gpuPct := stats.UtilPct
		var vramPct float64
		if stats.TotalBytes > 0 {
			vramPct = float64(stats.UsedBytes) * 100 / float64(stats.TotalBytes)
		}

		cpuPct, cpuRepeats := metricCaches[0].Update(turn, cpuPct)
		ramPct, ramRepeats := metricCaches[1].Update(turn, ramPct)
		gpuPct, gpuRepeats := metricCaches[2].Update(turn, gpuPct)
		vramPct, vramRepeats := metricCaches[3].Update(turn, vramPct)

		for i, ch := range charts {
			if ch == nil {
				continue
			}
			var value float64
			var repeats int
			switch i {
			case 0:
				value = cpuPct
				repeats = cpuRepeats
			case 1:
				value = ramPct
				repeats = ramRepeats
			case 2:
				value = gpuPct
				repeats = gpuRepeats
			case 3:
				value = vramPct
				repeats = vramRepeats
			}
			if repeats == 0 {
				continue
			}
			dirtyRegions := ch.UpdateRepeated(value, repeats)
			chart.ApplyRegions(screenFrame, dirtyRegions)
			for _, r := range dirtyRegions {
				if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
					r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
					return err
				}
			}
		}

		dbg.Printf("Refreshed: CPU=%.0f%% RAM=%.0f%% GPU=%.0f%% VRAM=%.0f%%",
			cpuPct, ramPct, gpuPct, vramPct)
		return nil
	}

	recoverStats := func(err error) (bool, error) {
		if !win.IsRetryablePdhError(err) {
			return false, err
		}
		dbg.Printf("GPU stats query failed with retryable PDH error: %v", err)
		rebuilt, rebuildErr := win.NewGpuPdhQuery()
		if rebuildErr != nil {
			dbg.Printf("GPU stats query rebuild failed: %v", rebuildErr)
			return false, rebuildErr
		}
		oldQuery := statsQuery
		statsQuery = rebuilt
		oldQuery.Close()
		dbg.Printf("GPU stats query rebuilt after PDH error.")
		return true, nil
	}

	turn := 0
	if err := updateStats(turn); err != nil {
		if recovered, _ := recoverStats(err); recovered {
			err = updateStats(turn)
		}
		if err != nil {
			dbg.Printf("Error: %v", err)
			fatalApp(err, nil)
		}
	}
	dbg.Printf("Running... (Ctrl+C to stop)")

	for {
		if shouldStop(interrupt, win.ExitApp()) {
			dbg.Printf("Exit via tray menu.")
			return
		}

		time.Sleep(1 * time.Second)
		turn++
		if err := updateStats(turn); err != nil {
			if win.IsRetryablePdhError(err) {
				if recovered, _ := recoverStats(err); recovered {
					if err = updateStats(turn); err == nil {
						continue
					}
					dbg.Printf("GPU stats refresh still failing after rebuild: %v", err)
				}
				continue
			}
			dbg.Printf("Error: %v", err)
			fatalApp(err, nil)
		}
	}
}
