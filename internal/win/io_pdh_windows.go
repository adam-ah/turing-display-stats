//go:build windows

package win

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type NetworkDiskStats struct {
	NetworkDownloadBytesPerSec float64
	NetworkUploadBytesPerSec   float64
	DiskReadActivePct          float64
	DiskWriteActivePct         float64
}

type NetworkDiskPdhQuery struct {
	handle      windows.Handle
	download    windows.Handle
	upload      windows.Handle
	diskRead    windows.Handle
	diskWrite   windows.Handle
	hasNetwork  bool
	hasDisk     bool
	initialized bool
}

func NewNetworkDiskPdhQuery(includeNetwork, includeDisk bool) (*NetworkDiskPdhQuery, error) {
	if !includeNetwork && !includeDisk {
		return nil, fmt.Errorf("network/disk PDH query needs at least one counter family")
	}

	var h windows.Handle
	status, _, _ := pdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&h)))
	if status != 0 {
		return nil, fmt.Errorf("PdhOpenQueryW: 0x%08X", status)
	}

	q := &NetworkDiskPdhQuery{handle: h}
	if includeNetwork {
		if err := q.addCounter(`\Network Interface(*)\Bytes Received/sec`, &q.download); err != nil {
			q.Close()
			return nil, err
		}
		if err := q.addCounter(`\Network Interface(*)\Bytes Sent/sec`, &q.upload); err != nil {
			q.Close()
			return nil, err
		}
		q.hasNetwork = true
	}
	if includeDisk {
		if err := q.addCounter(`\PhysicalDisk(*)\% Disk Read Time`, &q.diskRead); err != nil {
			q.Close()
			return nil, err
		}
		if err := q.addCounter(`\PhysicalDisk(*)\% Disk Write Time`, &q.diskWrite); err != nil {
			q.Close()
			return nil, err
		}
		q.hasDisk = true
	}

	if err := q.collect(); err != nil {
		q.Close()
		return nil, err
	}
	q.initialized = true
	return q, nil
}

func (q *NetworkDiskPdhQuery) addCounter(path string, out *windows.Handle) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	status, _, _ := pdhAddEnglishCounterW.Call(
		uintptr(q.handle),
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(out)),
	)
	if status != 0 {
		return fmt.Errorf("PdhAddEnglishCounterW(%s): 0x%08X", path, status)
	}
	return nil
}

func (q *NetworkDiskPdhQuery) collect() error {
	status, _, _ := pdhCollectQueryData.Call(uintptr(q.handle))
	if status != 0 {
		return fmt.Errorf("PdhCollectQueryData: 0x%08X", status)
	}
	return nil
}

func (q *NetworkDiskPdhQuery) Close() {
	if q.handle != 0 {
		pdhCloseQuery.Call(uintptr(q.handle))
		q.handle = 0
	}
}

func (q *NetworkDiskPdhQuery) formattedArray(counter windows.Handle, format uint32) ([]pdhFmtCounterValueItem, error) {
	var bufSize uint32
	var itemCount uint32

	status, _, _ := pdhGetFormattedArrayW.Call(
		uintptr(counter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		0,
	)
	if status != pdhMoreData {
		if status == 0 {
			return nil, nil
		}
		return nil, PdhError{Op: "PdhGetFormattedCounterArrayW(size)", Status: uintptr(status)}
	}

	buf := make([]byte, bufSize)
	status, _, _ = pdhGetFormattedArrayW.Call(
		uintptr(counter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if status != 0 {
		return nil, PdhError{Op: "PdhGetFormattedCounterArrayW(data)", Status: uintptr(status)}
	}

	items := make([]pdhFmtCounterValueItem, 0, itemCount)
	itemSize := unsafe.Sizeof(pdhFmtCounterValueItem{})
	base := unsafe.Pointer(&buf[0])
	for i := uint32(0); i < itemCount; i++ {
		item := (*pdhFmtCounterValueItem)(unsafe.Add(base, uintptr(i)*itemSize))
		items = append(items, *item)
	}
	return items, nil
}

func aggregateCounterItems(items []pdhFmtCounterValueItem) float64 {
	var total float64
	var hasTotal bool
	for _, item := range items {
		name := strings.ToLower(windows.UTF16PtrToString(item.Name))
		if name == "_total" {
			return item.FmtValue.asFloat64()
		}
		total += item.FmtValue.asFloat64()
		hasTotal = true
	}
	if !hasTotal {
		return 0
	}
	return total
}

func (q *NetworkDiskPdhQuery) Snapshot() (NetworkDiskStats, error) {
	if !q.initialized {
		return NetworkDiskStats{}, fmt.Errorf("PDH query not initialized")
	}
	if err := q.collect(); err != nil {
		return NetworkDiskStats{}, err
	}

	var stats NetworkDiskStats
	if q.hasNetwork {
		downloadItems, err := q.formattedArray(q.download, pdhFmtDouble)
		if err != nil {
			return NetworkDiskStats{}, err
		}
		uploadItems, err := q.formattedArray(q.upload, pdhFmtDouble)
		if err != nil {
			return NetworkDiskStats{}, err
		}
		stats.NetworkDownloadBytesPerSec = aggregateCounterItems(downloadItems)
		stats.NetworkUploadBytesPerSec = aggregateCounterItems(uploadItems)
	}
	if q.hasDisk {
		readItems, err := q.formattedArray(q.diskRead, pdhFmtDouble)
		if err != nil {
			return NetworkDiskStats{}, err
		}
		writeItems, err := q.formattedArray(q.diskWrite, pdhFmtDouble)
		if err != nil {
			return NetworkDiskStats{}, err
		}
		stats.DiskReadActivePct = aggregateCounterItems(readItems)
		stats.DiskWriteActivePct = aggregateCounterItems(writeItems)
	}

	return stats, nil
}
