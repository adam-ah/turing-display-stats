//go:build windows

// turing-display-go: Communicate with Turing Smart Screen displays from Go.
// Build: go build -trimpath -ldflags="-H windowsgui -s -w -buildid=" -o turing-display.exe
package app

import (
	"errors"
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

type runtimeMetric struct {
	name   string
	graph  chart.GraphConfig
	cache  sampler.MetricCache
	sCache sampler.SeriesMetricCache
	single *chart.Chart
	series *chart.SeriesChart
}

type pdhMetricSource string

const (
	pdhMetricGPU pdhMetricSource = "gpu"
	pdhMetricIO  pdhMetricSource = "io"
)

type pdhMetricError struct {
	source pdhMetricSource
	err    error
}

func (e pdhMetricError) Error() string {
	return e.err.Error()
}

func (e pdhMetricError) Unwrap() error {
	return e.err
}

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
	defer func() {
		win.CloseSerial(handle)
	}()
	dbg.Printf("Serial port opened.")

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

	var hasNetworkGraph, hasDiskGraph bool
	var ioQuery *win.NetworkDiskPdhQuery
	if chartCfg != nil {
		_, hasNetworkGraph = chartCfg.Graphs["network"]
		_, hasDiskGraph = chartCfg.Graphs["disk"]
		if hasNetworkGraph || hasDiskGraph {
			ioQuery, err = win.NewNetworkDiskPdhQuery(hasNetworkGraph, hasDiskGraph)
			if err != nil {
				dbg.Printf("Error: %v", err)
				fatalApp(err, nil)
			}
			defer ioQuery.Close()
		}
	}

	cpuSampler, err := win.NewCpuUsageSampler()
	if err != nil {
		dbg.Printf("Error: %v", err)
		fatalApp(err, nil)
	}

	screenCfg := defaultScreen()
	blocks := defaultBlocks()
	fontColor := color.RGBA{0, 0, 0, 255}
	bgColor := color.RGBA{255, 255, 255, 255}
	if chartCfg != nil {
		screenCfg = chartCfg.Screen
		fontColor = chartCfg.Colors.FontColor.RGBA
		bgColor = chartCfg.Colors.Background.RGBA
		if len(chartCfg.Blocks) > 0 {
			blocks = chartCfg.Blocks
		}
	}
	if screenCfg.Width <= 0 || screenCfg.Height <= 0 {
		screenCfg = defaultScreen()
	}
	base := renderBaseFrame(screenCfg, blocks, fontColor, bgColor)
	screenFrame := frame.NewScreenFrame(base)

	sendFullFrame := func() error {
		return win.SendDisplayBitmapRevA(handle, 0, 0, screenCfg.Width-1, screenCfg.Height-1, screenFrame)
	}

	metrics := make([]runtimeMetric, 0, len(blocks))
	for _, block := range blocks {
		if chartCfg == nil {
			continue
		}
		gCfg, ok := chartCfg.Graphs[block.Metric]
		if !ok {
			continue
		}
		rt := runtimeMetric{
			name:  block.Metric,
			graph: gCfg,
			cache: sampler.NewMetricCache(gCfg.RefreshSec),
		}
		switch block.Metric {
		case "network", "disk":
			rt.series = chart.NewSeriesChart(gCfg, chartCfg.DotSize, chartCfg.Colors.Background.RGBA, true)
			rt.sCache = sampler.NewSeriesMetricCache(gCfg.RefreshSec)
			dbg.Printf("%s chart: %dx%d at (%d,%d), capacity=%d",
				block.Metric, gCfg.Width, gCfg.Height, gCfg.X, gCfg.Y, rt.series.Capacity())
		default:
			rt.single = chart.NewChart(gCfg, chartCfg.DotSize, chartCfg.Thresholds, chartCfg.Colors, chartCfg.Colors.Background.RGBA, gCfg.RefreshSec, true)
			dbg.Printf("%s chart: %dx%d at (%d,%d), capacity=%d",
				block.Metric, gCfg.Width, gCfg.Height, gCfg.X, gCfg.Y, rt.single.Capacity())
		}
		metrics = append(metrics, rt)
	}

	syncDisplay := func() error {
		dbg.Printf("Sending HELLO handshake...")
		resp, err := win.SendHello(handle)
		if err != nil {
			return fmt.Errorf("send HELLO: %w", err)
		}
		dbg.Printf("Response (%d bytes): %s", len(resp), win.InterpretHello(resp))
		if err := sendFullFrame(); err != nil {
			return fmt.Errorf("send base frame: %w", err)
		}
		dbg.Printf("Base frame sent.")
		return nil
	}

	reconnectDisplay := func() error {
		const reconnectAttempts = 3
		const reconnectDelay = 5 * time.Second

		oldHandle := handle
		handle = 0
		win.CloseSerial(oldHandle)

		var lastErr error
		for attempt := 1; attempt <= reconnectAttempts; attempt++ {
			time.Sleep(reconnectDelay)

			dbg.Printf("Reopening serial port after display write failure (attempt %d/%d)...", attempt, reconnectAttempts)
			newHandle, openErr := win.OpenSerial(comPort)
			if openErr != nil {
				lastErr = openErr
				dbg.Printf("Error: %v", openErr)
				continue
			}

			handle = newHandle
			if syncErr := syncDisplay(); syncErr != nil {
				lastErr = syncErr
				dbg.Printf("Error: %v", syncErr)
				win.CloseSerial(newHandle)
				handle = 0
				continue
			}

			dbg.Printf("Display reconnected and resynced.")
			return nil
		}

		if lastErr == nil {
			lastErr = fmt.Errorf("display reconnect failed without a specific error")
		}
		return fmt.Errorf("reconnect display after write failure: %w", lastErr)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	recoverGpuStats := func(err error) (bool, error) {
		var metricErr pdhMetricError
		if !errors.As(err, &metricErr) || metricErr.source != pdhMetricGPU || !win.IsRetryablePdhError(err) {
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

	recoverIOStats := func(err error) (bool, error) {
		var metricErr pdhMetricError
		if ioQuery == nil || !errors.As(err, &metricErr) || metricErr.source != pdhMetricIO || !win.IsRetryablePdhError(err) {
			return false, err
		}
		dbg.Printf("Network/disk stats query failed with retryable PDH error: %v", err)
		rebuilt, rebuildErr := win.NewNetworkDiskPdhQuery(hasNetworkGraph, hasDiskGraph)
		if rebuildErr != nil {
			dbg.Printf("Network/disk stats query rebuild failed: %v", rebuildErr)
			return false, rebuildErr
		}
		oldQuery := ioQuery
		ioQuery = rebuilt
		oldQuery.Close()
		dbg.Printf("Network/disk stats query rebuilt after PDH error.")
		return true, nil
	}

	updateStats := func(turn int) error {
		stats, err := statsQuery.Snapshot()
		if err != nil {
			return pdhMetricError{source: pdhMetricGPU, err: err}
		}
		var ioStats win.NetworkDiskStats
		if ioQuery != nil {
			ioStats, err = ioQuery.Snapshot()
			if err != nil {
				return pdhMetricError{source: pdhMetricIO, err: err}
			}
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

		var ioDownPct, ioUpPct, ioDiskReadPct, ioDiskWritePct float64

		for i := range metrics {
			m := &metrics[i]
			switch m.name {
			case "cpu":
				value, repeats := m.cache.Update(turn, cpuPct)
				if repeats == 0 || m.single == nil {
					continue
				}
				dirtyRegions := m.single.UpdateRepeated(value, repeats)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			case "ram":
				value, repeats := m.cache.Update(turn, ramPct)
				if repeats == 0 || m.single == nil {
					continue
				}
				dirtyRegions := m.single.UpdateRepeated(value, repeats)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			case "gpu":
				value, repeats := m.cache.Update(turn, gpuPct)
				if repeats == 0 || m.single == nil {
					continue
				}
				dirtyRegions := m.single.UpdateRepeated(value, repeats)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			case "vram":
				value, repeats := m.cache.Update(turn, vramPct)
				if repeats == 0 || m.single == nil {
					continue
				}
				dirtyRegions := m.single.UpdateRepeated(value, repeats)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			case "network":
				ioDownPct = sampler.NormalizeBytesPerSec(ioStats.NetworkDownloadBytesPerSec, m.graph.MaxBytesPerSec)
				ioUpPct = sampler.NormalizeBytesPerSec(ioStats.NetworkUploadBytesPerSec, m.graph.MaxBytesPerSec)
				samples := m.sCache.Update(turn, []float64{ioDownPct, ioUpPct})
				if len(samples) == 0 || m.series == nil {
					continue
				}
				dirtyRegions := m.series.UpdateSamples(samples)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			case "disk":
				ioDiskReadPct = sampler.NormalizeDiskActiveTimePct(ioStats.DiskReadActivePct)
				ioDiskWritePct = sampler.NormalizeDiskActiveTimePct(ioStats.DiskWriteActivePct)
				samples := m.sCache.Update(turn, []float64{ioDiskReadPct, ioDiskWritePct})
				if len(samples) == 0 || m.series == nil {
					continue
				}
				dirtyRegions := m.series.UpdateSamples(samples)
				chart.ApplyRegions(screenFrame, dirtyRegions)
				for _, r := range dirtyRegions {
					if err := win.SendDisplayBitmapRevA(handle, r.X, r.Y,
						r.X+r.Image.Bounds().Dx()-1, r.Y+r.Image.Bounds().Dy()-1, r.Image); err != nil {
						return err
					}
				}
			}
		}

		dbg.Printf("Refreshed: CPU=%.0f%% RAM=%.0f%% GPU=%.0f%% VRAM=%.0f%% NET=%.0f%%/%.0f%% DISK=%.0f%%/%.0f%%",
			cpuPct, ramPct, gpuPct, vramPct, ioDownPct, ioUpPct, ioDiskReadPct, ioDiskWritePct)
		return nil
	}

	turn := 0
	if err := syncDisplay(); err != nil {
		dbg.Printf("Error: %v", err)
		if isRecoverableDisplayWriteError(err) {
			dbg.Printf("Display write failed with recoverable error: %v", err)
			if reconnectErr := reconnectDisplay(); reconnectErr != nil {
				dbg.Printf("Error: %v", reconnectErr)
				fatalApp(reconnectErr, nil)
			}
		} else {
			fatalApp(err, nil)
		}
	}

	if err := updateStats(turn); err != nil {
		if isRecoverableDisplayWriteError(err) {
			dbg.Printf("Display write failed with recoverable error: %v", err)
			if reconnectErr := reconnectDisplay(); reconnectErr != nil {
				dbg.Printf("Error: %v", reconnectErr)
				fatalApp(reconnectErr, nil)
			}
		} else if recovered, _ := recoverGpuStats(err); recovered {
			err = updateStats(turn)
			if err != nil {
				dbg.Printf("Error: %v", err)
				fatalApp(err, nil)
			}
		} else if recovered, _ := recoverIOStats(err); recovered {
			err = updateStats(turn)
			if err != nil {
				dbg.Printf("Error: %v", err)
				fatalApp(err, nil)
			}
		} else {
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
			if isRecoverableDisplayWriteError(err) {
				dbg.Printf("Display write failed with recoverable error: %v", err)
				if reconnectErr := reconnectDisplay(); reconnectErr != nil {
					dbg.Printf("Error: %v", reconnectErr)
					fatalApp(reconnectErr, nil)
				}
				continue
			}
			if win.IsRetryablePdhError(err) {
				if recovered, _ := recoverGpuStats(err); recovered {
					if err = updateStats(turn); err == nil {
						continue
					}
					dbg.Printf("GPU stats refresh still failing after rebuild: %v", err)
				} else if recovered, _ := recoverIOStats(err); recovered {
					if err = updateStats(turn); err == nil {
						continue
					}
					dbg.Printf("Network/disk stats refresh still failing after rebuild: %v", err)
				}
				continue
			}
			dbg.Printf("Error: %v", err)
			fatalApp(err, nil)
		}
	}
}
