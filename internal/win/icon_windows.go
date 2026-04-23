//go:build windows

package win

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"unsafe"
)

//go:embed assets/tray.ico
var trayIconData []byte

type icoHeader struct {
	Reserved uint16
	Type     uint16
	Count    uint16
}

type icoDirEntry struct {
	Width       byte
	Height      byte
	ColorCount  byte
	Reserved    byte
	Planes      uint16
	BitCount    uint16
	BytesInRes  uint32
	ImageOffset uint32
}

func loadTrayIcon() (uintptr, error) {
	entry, err := bestIcoEntry(trayIconData)
	if err != nil {
		return 0, err
	}
	if int(entry.ImageOffset)+int(entry.BytesInRes) > len(trayIconData) {
		return 0, fmt.Errorf("tray icon data is truncated")
	}
	iconBits := trayIconData[entry.ImageOffset : entry.ImageOffset+entry.BytesInRes]
	cx := iconEntrySize(entry.Width)
	cy := iconEntrySize(entry.Height)
	icon, _, errCall := procCreateIconFromResEx.Call(
		uintptr(unsafe.Pointer(&iconBits[0])),
		uintptr(len(iconBits)),
		1,
		0x00030000,
		uintptr(cx),
		uintptr(cy),
		0,
	)
	if icon == 0 {
		return 0, fmt.Errorf("CreateIconFromResourceEx failed: %v", errCall)
	}
	return icon, nil
}

func bestIcoEntry(data []byte) (icoDirEntry, error) {
	if len(data) < binary.Size(icoHeader{}) {
		return icoDirEntry{}, fmt.Errorf("tray icon data too short")
	}

	var hdr icoHeader
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &hdr); err != nil {
		return icoDirEntry{}, err
	}
	if hdr.Reserved != 0 || hdr.Type != 1 || hdr.Count == 0 {
		return icoDirEntry{}, fmt.Errorf("tray icon data is not a valid ICO")
	}

	reader := bytes.NewReader(data[binary.Size(icoHeader{}):])
	entries := make([]icoDirEntry, hdr.Count)
	if err := binary.Read(reader, binary.LittleEndian, &entries); err != nil {
		return icoDirEntry{}, err
	}

	best := entries[0]
	bestScore := icoEntryScore(best)
	for _, entry := range entries[1:] {
		score := icoEntryScore(entry)
		if score > bestScore {
			best = entry
			bestScore = score
		}
	}
	return best, nil
}

func icoEntryScore(entry icoDirEntry) int {
	w := iconEntrySize(entry.Width)
	h := iconEntrySize(entry.Height)
	return w*h*1000 + int(entry.BitCount)
}

func iconEntrySize(v byte) int {
	if v == 0 {
		return 256
	}
	return int(v)
}
