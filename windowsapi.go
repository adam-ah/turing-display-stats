//go:build windows

// Windows API wrappers: serial port, GPU PDH, DXGI, system memory, CPU usage.
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"log"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------------------------------------------------------------------------
// Device signatures
// ---------------------------------------------------------------------------

type deviceSig struct {
	vid, pid uint16
	name     string
}

var knownDevices = []deviceSig{
	{0x1a86, 0x5722, "Turing Smart Screen / UsbPCMonitor (Rev A/B)"},
	{0x454d, 0x4e41, "Kipye Qiye Smart Display (Rev D)"},
}

// ---------------------------------------------------------------------------
// Registry helpers
// ---------------------------------------------------------------------------

const (
	dcbFlagBinary      = 1 << 0
	dcbFlagParity      = 1 << 1
	dcbFlagOutxCtsFlow = 1 << 2
	dcbDtrControlShift = 4
	dcbRtsControlShift = 12
)

var portsClassGUID = windows.GUID{
	Data1: 0x4d36e978,
	Data2: 0xe325,
	Data3: 0x11ce,
	Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18},
}

type comPortInfo struct {
	path     string
	portName string
	vid      uint16
	pid      uint16
}

func enumSerialPorts() ([]comPortInfo, error) {
	devInfo, err := windows.SetupDiGetClassDevsEx(&portsClassGUID, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, fmt.Errorf("SetupDiGetClassDevsEx(Ports): %w", err)
	}
	defer devInfo.Close()

	var ports []comPortInfo
	for i := 0; ; i++ {
		dev, err := devInfo.EnumDeviceInfo(i)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			continue
		}

		friendly, _ := devInfo.DeviceRegistryProperty(dev, windows.SPDRP_FRIENDLYNAME)
		if friendly == nil {
			friendly, _ = devInfo.DeviceRegistryProperty(dev, windows.SPDRP_DEVICEDESC)
		}
		friendlyName, _ := friendly.(string)
		portName := parseCOMPort(friendlyName)
		if portName == "" {
			continue
		}

		hardware, _ := devInfo.DeviceRegistryProperty(dev, windows.SPDRP_HARDWAREID)
		vid, pid := parseVidPid(hardware)
		ports = append(ports, comPortInfo{
			path:     friendlyName,
			portName: portName,
			vid:      vid,
			pid:      pid,
		})
	}
	return ports, nil
}

func parseCOMPort(s string) string {
	start := strings.LastIndex(s, "(COM")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], ")")
	if end < 0 {
		return ""
	}
	port := s[start+1 : start+end]
	if !strings.HasPrefix(port, "COM") {
		return ""
	}
	if _, err := strconv.Atoi(strings.TrimPrefix(port, "COM")); err != nil {
		return ""
	}
	return `\\.\` + port
}

func parseVidPid(v interface{}) (uint16, uint16) {
	var text string
	switch x := v.(type) {
	case string:
		text = x
	case []string:
		text = strings.Join(x, " ")
	default:
		return 0, 0
	}
	upper := strings.ToUpper(text)
	var vid, pid uint16
	if i := strings.Index(upper, "VID_"); i >= 0 && i+8 <= len(upper) {
		fmt.Sscanf(upper[i+4:i+8], "%X", &vid)
	}
	if i := strings.Index(upper, "PID_"); i >= 0 && i+8 <= len(upper) {
		fmt.Sscanf(upper[i+4:i+8], "%X", &pid)
	}
	return vid, pid
}

func findTuringDisplay() (comPort, deviceName string, err error) {
	ports, err := enumSerialPorts()
	if err != nil {
		return "", "", fmt.Errorf("enumerate serial ports: %w", err)
	}
	if len(ports) == 0 {
		return "", "", fmt.Errorf("no COM ports found")
	}

	for _, port := range ports {
		for _, dev := range knownDevices {
			if port.vid == dev.vid && port.pid == dev.pid {
				return port.portName, dev.name, nil
			}
		}
	}

	return "", "", fmt.Errorf("no Turing display found — checked %d COM port(s), "+
		"looking for VID/PID: %s", len(ports), sigList())
}

func sigList() string {
	var parts []string
	for _, d := range knownDevices {
		parts = append(parts, fmt.Sprintf("VID_%04X PID_%04X (%s)", d.vid, d.pid, d.name))
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Serial port
// ---------------------------------------------------------------------------

func openSerial(portName string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(portName)
	if err != nil {
		return 0, err
	}

	handle, err := windows.CreateFile(
		p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateFile(%s): %w", portName, err)
	}

	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(handle, &dcb); err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("GetCommState: %w", err)
	}

	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	dcb.BaudRate = 115200
	dcb.ByteSize = 8
	dcb.Parity = windows.NOPARITY
	dcb.StopBits = windows.ONESTOPBIT
	dcb.Flags &^= dcbFlagParity | dcbFlagOutxCtsFlow
	dcb.Flags &^= 0x3 << dcbDtrControlShift
	dcb.Flags &^= 0x3 << dcbRtsControlShift
	dcb.Flags |= dcbFlagBinary | dcbFlagOutxCtsFlow | windows.DTR_CONTROL_ENABLE | windows.RTS_CONTROL_HANDSHAKE

	if err := windows.SetCommState(handle, &dcb); err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("SetCommState: %w", err)
	}

	if err := windows.SetupComm(handle, 65536, 65536); err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("SetupComm: %w", err)
	}

	timeouts := windows.CommTimeouts{
		ReadIntervalTimeout:         0xFFFFFFFF,
		ReadTotalTimeoutMultiplier:  0,
		ReadTotalTimeoutConstant:    1000,
		WriteTotalTimeoutMultiplier: 0,
		WriteTotalTimeoutConstant:   1000,
	}
	if err := windows.SetCommTimeouts(handle, &timeouts); err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("SetCommTimeouts: %w", err)
	}

	windows.PurgeComm(handle, windows.PURGE_TXCLEAR|windows.PURGE_RXCLEAR)
	return handle, nil
}

func closeSerial(handle windows.Handle) {
	windows.CloseHandle(handle)
}

// ---------------------------------------------------------------------------
// HELLO handshake
// ---------------------------------------------------------------------------

const helloCmd = 0x45

func sendHello(handle windows.Handle) ([]byte, error) {
	helloPacket := []byte{helloCmd, helloCmd, helloCmd, helloCmd, helloCmd, helloCmd}

	var written uint32
	if err := windows.WriteFile(handle, helloPacket, &written, nil); err != nil {
		return nil, fmt.Errorf("write HELLO: %w", err)
	}
	if written != uint32(len(helloPacket)) {
		return nil, fmt.Errorf("write HELLO: short write %d/%d", written, len(helloPacket))
	}

	resp := make([]byte, 6)
	var n uint32
	err := windows.ReadFile(handle, resp, &n, nil)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return resp[:n], nil
}

// ---------------------------------------------------------------------------
// Display bitmap
// ---------------------------------------------------------------------------

func imageToRGB565LE(img image.Image) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := make([]byte, 0, w*h*2)
	var px [2]byte
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r5 := uint16(r >> 11)
			g6 := uint16(g >> 10)
			b5 := uint16(b >> 11)
			pixel := (r5 << 11) | (g6 << 5) | b5
			binary.LittleEndian.PutUint16(px[:], pixel)
			out = append(out, px[:]...)
		}
	}
	return out
}

func sendDisplayBitmapRevA(handle windows.Handle, x0, y0, x1, y1 int, img image.Image) error {
	header := []byte{
		byte(x0 >> 2),
		byte(((x0 & 3) << 6) | (y0 >> 4)),
		byte(((y0 & 15) << 4) | (x1 >> 6)),
		byte(((x1 & 63) << 2) | (y1 >> 8)),
		byte(y1 & 255),
		0xC5,
	}

	var written uint32
	if err := windows.WriteFile(handle, header, &written, nil); err != nil {
		return fmt.Errorf("write display header: %w", err)
	}
	if written != uint32(len(header)) {
		return fmt.Errorf("write display header: short write %d/%d", written, len(header))
	}

	rgb565 := imageToRGB565LE(img)
	chunkSize := 320 * 8
	for i := 0; i < len(rgb565); i += chunkSize {
		end := i + chunkSize
		if end > len(rgb565) {
			end = len(rgb565)
		}
		if err := windows.WriteFile(handle, rgb565[i:end], &written, nil); err != nil {
			return fmt.Errorf("write display payload: %w", err)
		}
		if written != uint32(end-i) {
			return fmt.Errorf("write display payload: short write %d/%d", written, end-i)
		}
	}

	time.Sleep(50 * time.Millisecond)
	return nil
}

// ---------------------------------------------------------------------------
// GPU PDH + DXGI
// ---------------------------------------------------------------------------

const (
	pdhFmtDouble = 0x00000200
	pdhFmtLarge  = 0x00000400
	pdhMoreData  = 0x800007D2
)

var (
	pdhDLL                = windows.NewLazySystemDLL("pdh.dll")
	pdhOpenQueryW         = pdhDLL.NewProc("PdhOpenQueryW")
	pdhAddEnglishCounterW = pdhDLL.NewProc("PdhAddEnglishCounterW")
	pdhCollectQueryData   = pdhDLL.NewProc("PdhCollectQueryData")
	pdhGetFormattedArrayW = pdhDLL.NewProc("PdhGetFormattedCounterArrayW")
	pdhCloseQuery         = pdhDLL.NewProc("PdhCloseQuery")
	dxgiDLL               = windows.NewLazySystemDLL("dxgi.dll")
	createDXGIFactory1    = dxgiDLL.NewProc("CreateDXGIFactory1")
)

type gpuStats struct {
	usedBytes  uint64
	totalBytes uint64
	utilPct    float64
}

type dxgiQueryVideoMemoryInfo struct {
	Budget                  uint64
	CurrentUsage            uint64
	AvailableForReservation uint64
	CurrentReservation      uint64
}

type pdhFmtCounterValue struct {
	CStatus    uint32
	_          uint32
	LargeValue int64
}

type pdhFmtCounterValueItem struct {
	Name     *uint16
	FmtValue pdhFmtCounterValue
}

type gpuPdhQuery struct {
	handle      windows.Handle
	memory      windows.Handle
	engine      windows.Handle
	totalBytes  uint64
	initialized bool
}

type dxgiFactory1 struct {
	lpVtbl *dxgiFactory1Vtbl
}

type dxgiFactory1Vtbl struct {
	queryInterface        uintptr
	addRef                uintptr
	release               uintptr
	setPrivateData        uintptr
	setPrivateDataIface   uintptr
	getPrivateData        uintptr
	getParent             uintptr
	enumAdapters          uintptr
	makeWindowAssociation uintptr
	getWindowAssociation  uintptr
	createSwapChain       uintptr
	createSoftwareAdapter uintptr
	enumAdapters1         uintptr
	isCurrent             uintptr
}

type dxgiAdapter struct {
	lpVtbl *dxgiAdapterVtbl
}

type dxgiAdapterVtbl struct {
	queryInterface      uintptr
	addRef              uintptr
	release             uintptr
	setPrivateData      uintptr
	setPrivateDataIface uintptr
	getPrivateData      uintptr
	getParent           uintptr
	enumOutputs         uintptr
	getDesc             uintptr
}

type dxgiAdapterDesc struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uint64
	DedicatedSystemMemory uint64
	SharedSystemMemory    uint64
	AdapterLuid           windows.LUID
}

type dxgiAdapter3 struct {
	lpVtbl *dxgiAdapter3Vtbl
}

type dxgiAdapter3Vtbl struct {
	queryInterface            uintptr
	addRef                    uintptr
	release                   uintptr
	setPrivateData            uintptr
	setPrivateDataIface       uintptr
	getPrivateData            uintptr
	getParent                 uintptr
	enumOutputs               uintptr
	getDesc                   uintptr
	checkInterfaceSupport     uintptr
	getDesc1                  uintptr
	getDesc2                  uintptr
	registerHardwareEvent     uintptr
	unregisterHardwareEvent   uintptr
	queryVideoMemoryInfo      uintptr
	setVideoMemoryReservation uintptr
	registerBudgetChangeEvent uintptr
	unregisterBudgetChange    uintptr
}

func newGpuPdhQuery() (*gpuPdhQuery, error) {
	var h windows.Handle
	status, _, _ := pdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&h)))
	if status != 0 {
		return nil, fmt.Errorf("PdhOpenQueryW: 0x%08X", status)
	}

	q := &gpuPdhQuery{handle: h}
	if err := q.addCounter(`\GPU Adapter Memory(*)\Dedicated Usage`, &q.memory); err != nil {
		q.close()
		return nil, err
	}
	if err := q.addCounter(`\GPU Engine(*)\Utilization Percentage`, &q.engine); err != nil {
		q.close()
		return nil, err
	}

	totalBytes, err := readPrimaryAdapterDedicatedVideoMemory()
	if err != nil {
		q.close()
		return nil, err
	}
	q.totalBytes = totalBytes

	if err := q.collect(); err != nil {
		q.close()
		return nil, err
	}
	q.initialized = true
	return q, nil
}

func (q *gpuPdhQuery) addCounter(path string, out *windows.Handle) error {
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

func (q *gpuPdhQuery) collect() error {
	status, _, _ := pdhCollectQueryData.Call(uintptr(q.handle))
	if status != 0 {
		return fmt.Errorf("PdhCollectQueryData: 0x%08X", status)
	}
	return nil
}

func (q *gpuPdhQuery) close() {
	if q.handle != 0 {
		pdhCloseQuery.Call(uintptr(q.handle))
		q.handle = 0
	}
}

func (q *gpuPdhQuery) formattedArray(counter windows.Handle, format uint32) ([]pdhFmtCounterValueItem, error) {
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
		return nil, fmt.Errorf("PdhGetFormattedCounterArrayW(size): 0x%08X", status)
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
		return nil, fmt.Errorf("PdhGetFormattedCounterArrayW(data): 0x%08X", status)
	}

	items := make([]pdhFmtCounterValueItem, 0, itemCount)
	itemSize := unsafe.Sizeof(pdhFmtCounterValueItem{})
	base := uintptr(unsafe.Pointer(&buf[0]))
	for i := uint32(0); i < itemCount; i++ {
		item := (*pdhFmtCounterValueItem)(unsafe.Pointer(base + uintptr(i)*itemSize))
		items = append(items, *item)
	}
	return items, nil
}

func (v pdhFmtCounterValue) asFloat64() float64 {
	return *(*float64)(unsafe.Pointer(&v.LargeValue))
}

func (q *gpuPdhQuery) snapshot() (gpuStats, error) {
	if !q.initialized {
		return gpuStats{}, fmt.Errorf("PDH query not initialized")
	}
	if err := q.collect(); err != nil {
		return gpuStats{}, err
	}

	memItems, err := q.formattedArray(q.memory, pdhFmtLarge)
	if err != nil {
		return gpuStats{}, err
	}
	engineItems, err := q.formattedArray(q.engine, pdhFmtDouble)
	if err != nil {
		return gpuStats{}, err
	}

	var stats gpuStats
	if len(memItems) > 0 {
		stats.usedBytes = uint64(memItems[0].FmtValue.LargeValue)
	}
	stats.totalBytes = q.totalBytes

	log.Printf("[gpu] pdh memory samples: %d", len(memItems))
	for i, item := range memItems {
		name := windows.UTF16PtrToString(item.Name)
		val := uint64(item.FmtValue.LargeValue)
		log.Printf("[gpu]   mem[%d] %q = %s (%d)", i, name, formatBytesGiB(val), val)
	}

	var max3D float64
	log.Printf("[gpu] pdh engine samples: %d", len(engineItems))
	for _, item := range engineItems {
		name := windows.UTF16PtrToString(item.Name)
		val := item.FmtValue.asFloat64()
		log.Printf("[gpu]   eng %q = %.2f", name, val)
		if strings.Contains(strings.ToLower(name), "engtype_3d") && val > max3D {
			max3D = val
		}
	}
	stats.utilPct = max3D
	return stats, nil
}

func (f *dxgiFactory1) release() {
	if f != nil && f.lpVtbl != nil && f.lpVtbl.release != 0 {
		syscall.Syscall(f.lpVtbl.release, 1, uintptr(unsafe.Pointer(f)), 0, 0)
	}
}

func (f *dxgiFactory1) enumAdapters1(index uint32) (*dxgiAdapter, error) {
	var adapter *dxgiAdapter
	hr, _, _ := syscall.Syscall(f.lpVtbl.enumAdapters1, 3,
		uintptr(unsafe.Pointer(f)),
		uintptr(index),
		uintptr(unsafe.Pointer(&adapter)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("EnumAdapters1(%d): 0x%08X", index, hr)
	}
	return adapter, nil
}

func (a *dxgiAdapter) release() {
	if a != nil && a.lpVtbl != nil && a.lpVtbl.release != 0 {
		syscall.Syscall(a.lpVtbl.release, 1, uintptr(unsafe.Pointer(a)), 0, 0)
	}
}

func (a *dxgiAdapter) queryInterface(iid *windows.GUID, out unsafe.Pointer) error {
	hr, _, _ := syscall.Syscall(a.lpVtbl.queryInterface, 3,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(out),
	)
	if hr != 0 {
		return fmt.Errorf("QueryInterface: 0x%08X", hr)
	}
	return nil
}

func (a *dxgiAdapter) getDesc() (dxgiAdapterDesc, error) {
	var desc dxgiAdapterDesc
	hr, _, _ := syscall.Syscall(a.lpVtbl.getDesc, 2,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)
	if hr != 0 {
		return dxgiAdapterDesc{}, fmt.Errorf("GetDesc: 0x%08X", hr)
	}
	return desc, nil
}

func (a *dxgiAdapter3) release() {
	if a != nil && a.lpVtbl != nil && a.lpVtbl.release != 0 {
		syscall.Syscall(a.lpVtbl.release, 1, uintptr(unsafe.Pointer(a)), 0, 0)
	}
}

func readPrimaryAdapterDedicatedVideoMemory() (uint64, error) {
	coinited := false
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		if errno, ok := err.(syscall.Errno); !ok || errno != syscall.Errno(windows.RPC_E_CHANGED_MODE) {
			return 0, fmt.Errorf("CoInitializeEx: %w", err)
		}
	} else {
		coinited = true
	}
	if coinited {
		defer windows.CoUninitialize()
	}

	iidFactory1, err := windows.GUIDFromString("{770AAE78-F26F-4DBA-A829-253C83D1B387}")
	if err != nil {
		return 0, err
	}

	var factory *dxgiFactory1
	hr, _, _ := createDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if hr != 0 {
		return 0, fmt.Errorf("CreateDXGIFactory1: 0x%08X", hr)
	}
	defer factory.release()

	adapter, err := factory.enumAdapters1(0)
	if err != nil {
		return 0, err
	}
	defer adapter.release()

	iidAdapter3, err := windows.GUIDFromString("{645967A4-1392-4310-A798-8053CE3E93FD}")
	if err != nil {
		return 0, err
	}

	var adapter3 *dxgiAdapter3
	if err := adapter.queryInterface(&iidAdapter3, unsafe.Pointer(&adapter3)); err != nil {
		return 0, err
	}
	defer adapter3.release()

	desc, err := adapter.getDesc()
	if err != nil {
		return 0, err
	}
	adapterName := windows.UTF16ToString(desc.Description[:])

	var info dxgiQueryVideoMemoryInfo
	hr, _, _ = syscall.Syscall6(adapter3.lpVtbl.queryVideoMemoryInfo, 4,
		uintptr(unsafe.Pointer(adapter3)),
		0,
		0,
		uintptr(unsafe.Pointer(&info)),
		0,
		0,
	)
	if hr != 0 {
		return 0, fmt.Errorf("QueryVideoMemoryInfo: 0x%08X", hr)
	}

	log.Printf("[gpu] adapter=0 name=%q vendor=0x%04X device=0x%04X dedicated=%s shared=%s",
		adapterName, desc.VendorID, desc.DeviceID,
		formatBytesGiB(desc.DedicatedVideoMemory), formatBytesGiB(desc.SharedSystemMemory))
	log.Printf("[gpu] dxgi local budget=%s current=%s available=%s reservation=%s",
		formatBytesGiB(info.Budget),
		formatBytesGiB(info.CurrentUsage),
		formatBytesGiB(info.AvailableForReservation),
		formatBytesGiB(info.CurrentReservation))

	return desc.DedicatedVideoMemory, nil
}

// ---------------------------------------------------------------------------
// System memory + CPU
// ---------------------------------------------------------------------------

var (
	kernel32DLL          = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes       = kernel32DLL.NewProc("GetSystemTimes")
	globalMemoryStatusEx = kernel32DLL.NewProc("GlobalMemoryStatusEx")
)

type systemMemoryStats struct {
	usedBytes  uint64
	totalBytes uint64
	loadPct    float64
}

type fileTime struct {
	dwLowDateTime  uint32
	dwHighDateTime uint32
}

func (ft fileTime) uint64() uint64 {
	return uint64(ft.dwHighDateTime)<<32 | uint64(ft.dwLowDateTime)
}

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

type cpuUsageSampler struct {
	lastIdle    uint64
	lastKernel  uint64
	lastUser    uint64
	initialized bool
}

func readSystemMemoryStats() (systemMemoryStats, error) {
	var mem memoryStatusEx
	mem.dwLength = uint32(unsafe.Sizeof(mem))

	r1, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if r1 == 0 {
		return systemMemoryStats{}, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}

	return systemMemoryStats{
		usedBytes:  mem.ullTotalPhys - mem.ullAvailPhys,
		totalBytes: mem.ullTotalPhys,
		loadPct:    float64(mem.dwMemoryLoad),
	}, nil
}

func newCpuUsageSampler() (*cpuUsageSampler, error) {
	s := &cpuUsageSampler{}
	idle, kernel, user, err := s.readTimes()
	if err != nil {
		return nil, err
	}
	s.lastIdle = idle
	s.lastKernel = kernel
	s.lastUser = user
	s.initialized = true
	return s, nil
}

func (s *cpuUsageSampler) readTimes() (idle, kernel, user uint64, err error) {
	var idleFT, kernelFT, userFT fileTime
	r1, _, callErr := getSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	)
	if r1 == 0 {
		return 0, 0, 0, fmt.Errorf("GetSystemTimes: %w", callErr)
	}
	return idleFT.uint64(), kernelFT.uint64(), userFT.uint64(), nil
}

func (s *cpuUsageSampler) snapshot() (float64, error) {
	idle, kernel, user, err := s.readTimes()
	if err != nil {
		return 0, err
	}
	if !s.initialized {
		s.lastIdle = idle
		s.lastKernel = kernel
		s.lastUser = user
		s.initialized = true
		return 0, nil
	}

	if idle < s.lastIdle || kernel < s.lastKernel || user < s.lastUser {
		s.lastIdle = idle
		s.lastKernel = kernel
		s.lastUser = user
		return 0, nil
	}

	deltaIdle := idle - s.lastIdle
	deltaKernel := kernel - s.lastKernel
	deltaUser := user - s.lastUser
	deltaTotal := deltaKernel + deltaUser

	s.lastIdle = idle
	s.lastKernel = kernel
	s.lastUser = user

	if deltaTotal == 0 {
		return 0, nil
	}

	active := deltaTotal - deltaIdle
	if active > deltaTotal {
		active = 0
	}

	pct := float64(active) * 100 / float64(deltaTotal)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func formatBytesGiB(b uint64) string {
	return fmt.Sprintf("%.1f GiB", float64(b)/(1024*1024*1024))
}

func interpretHello(resp []byte) string {
	if len(resp) == 0 {
		return "No response received — device may be disconnected or on wrong baud rate"
	}

	allSame := true
	for i := 1; i < len(resp); i++ {
		if resp[i] != resp[0] {
			allSame = false
			break
		}
	}
	if allSame {
		switch resp[0] {
		case 0x01:
			return "USB Monitor 3.5\" (Rev A sub-revision)"
		case 0x02:
			return "USB Monitor 5\" (Rev C)"
		case 0x03:
			return "USB Monitor 7\""
		default:
			return fmt.Sprintf("USB Monitor variant, byte=0x%02X", resp[0])
		}
	}

	allZero := true
	for _, b := range resp {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return "Turing Smart Screen 3.5\" (standard, does not respond to HELLO — this is normal)"
	}

	var parts []string
	for _, b := range resp {
		parts = append(parts, fmt.Sprintf("0x%02X", b))
	}
	return "Mixed/unknown response: " + strings.Join(parts, " ")
}
