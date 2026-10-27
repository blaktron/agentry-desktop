package wasapi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Flow is an endpoint's direction.
type Flow uint32

const (
	// Render is an output: speakers, a headset's earpiece.
	Render Flow = 0
	// Capture is an input: a microphone.
	Capture Flow = 1
)

// Session is an app's streams on one endpoint, as the audio stack lists
// them.
type Session struct {
	Endpoint string
	Flow     Flow
	PID      uint32
	// Exe is the process's executable, App the description its version
	// resource gives ("Zoom Meetings"), "" where either cannot be read.
	Exe string
	App string
	// Active is a session with a stream running; an inactive one has
	// streams that are stopped.
	Active bool
}

// ErrGone is a stream whose endpoint went away or was reconfigured: a
// headset unplugged, a device switched to another profile.
var ErrGone = errors.New("the audio device went away")

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

const (
	clsctxAll         = 0x17
	deviceStateActive = 1
	eConsole          = 0
	coinitMTA         = 0x0
	stgmRead          = 0x0
	vtLPWSTR          = 31
	errChangedMode    = 0x80010106
	sOK               = 0
)

// comObj is a COM interface pointer: its first word is its table of
// functions.
type comObj struct {
	vtbl *[32]uintptr
}

func (o *comObj) call(slot int, args ...uintptr) uint32 {
	r, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return uint32(r)
}

func (o *comObj) release() {
	if o != nil {
		o.call(slotRelease)
	}
}

func failed(hr uint32) bool { return int32(hr) < 0 }

func hrError(what string, hr uint32) error {
	switch hr {
	case errDeviceInvalidated:
		return ErrGone
	case errAccessDenied:
		return fmt.Errorf("%s: Windows refused access (is \"Let desktop apps access your microphone\" off?)", what)
	}
	return fmt.Errorf("%s: HRESULT %#08x", what, hr)
}

func guid(s string) *windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return &g
}

// System is a connection to the audio stack. Open it, use it and close it
// from one goroutine: COM is initialised on the OS thread it locks.
type System struct {
	enum    *comObj
	consent registry.Key
	event   windows.Handle
	names   map[string]string
}

// Open initialises COM in the multithreaded apartment on this goroutine's
// thread and connects to the audio stack.
func Open() (*System, error) {
	runtime.LockOSThread()
	// S_FALSE, COM already initialised on this thread, is not a failure.
	if err := windows.CoInitializeEx(0, coinitMTA); err != nil {
		if e, ok := err.(syscall.Errno); !ok || (e != 1 && uint32(e) != errChangedMode) {
			runtime.UnlockOSThread()
			return nil, fmt.Errorf("COM: %w", err)
		}
	}
	s := &System{names: map[string]string{}}
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(guid(clsidMMDeviceEnumerator))), 0, clsctxAll,
		uintptr(unsafe.Pointer(guid(iidIMMDeviceEnumerator))), uintptr(unsafe.Pointer(&s.enum)))
	if failed(uint32(hr)) {
		windows.CoUninitialize()
		runtime.UnlockOSThread()
		return nil, hrError("the audio devices", uint32(hr))
	}
	s.watchConsent()
	return s, nil
}

// Close lets go of the audio stack and of COM.
func (s *System) Close() {
	s.enum.release()
	if s.event != 0 {
		_ = windows.CloseHandle(s.event)
	}
	if s.consent != 0 {
		_ = s.consent.Close()
	}
	windows.CoUninitialize()
	runtime.UnlockOSThread()
}

// consentKey is where Windows keeps which apps use the microphone and
// when; it changes as an app starts or stops recording.
const consentKey = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone`

func (s *System) watchConsent() {
	k, err := registry.OpenKey(registry.CURRENT_USER, consentKey, registry.NOTIFY)
	if err != nil {
		return
	}
	ev, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		_ = k.Close()
		return
	}
	s.consent, s.event = k, ev
	s.arm()
}

func (s *System) arm() {
	const filter = windows.REG_NOTIFY_CHANGE_NAME | windows.REG_NOTIFY_CHANGE_LAST_SET
	if windows.RegNotifyChangeKeyValue(windows.Handle(s.consent), true, filter, s.event, true) != nil {
		_ = windows.CloseHandle(s.event)
		_ = s.consent.Close()
		s.consent, s.event = 0, 0
	}
}

// Changed waits until an app starts or stops using a microphone, or for
// timeout, whichever is first. Where the change cannot be watched it waits
// for timeout.
func (s *System) Changed(timeout time.Duration) {
	if s.event == 0 {
		time.Sleep(timeout)
		return
	}
	if r, _ := windows.WaitForSingleObject(s.event, uint32(timeout/time.Millisecond)); r == windows.WAIT_OBJECT_0 {
		s.arm()
	}
}

// Default is the endpoint Windows uses by default for flow.
func (s *System) Default(flow Flow) (string, error) {
	var dev *comObj
	if hr := s.enum.call(slotGetDefaultAudioEndpoint, uintptr(flow), eConsole, uintptr(unsafe.Pointer(&dev))); failed(hr) {
		return "", hrError("the default device", hr)
	}
	defer dev.release()
	return deviceID(dev)
}

func deviceID(dev *comObj) (string, error) {
	var p *uint16
	if hr := dev.call(slotGetID, uintptr(unsafe.Pointer(&p))); failed(hr) {
		return "", hrError("a device's id", hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p), nil
}

// Endpoint is an active endpoint: its id, and the name Windows shows for
// it ("" where it cannot be read).
type Endpoint struct {
	ID   string
	Name string
}

// Endpoints lists the active endpoints of flow.
func (s *System) Endpoints(flow Flow) ([]Endpoint, error) {
	var coll *comObj
	if hr := s.enum.call(slotEnumAudioEndpoints, uintptr(flow), deviceStateActive, uintptr(unsafe.Pointer(&coll))); failed(hr) {
		return nil, hrError("the audio devices", hr)
	}
	defer coll.release()
	var n uint32
	coll.call(slotCollectionGetCount, uintptr(unsafe.Pointer(&n)))
	out := make([]Endpoint, 0, n)
	for i := range n {
		var dev *comObj
		if failed(coll.call(slotCollectionItem, uintptr(i), uintptr(unsafe.Pointer(&dev)))) {
			continue
		}
		id, err := deviceID(dev)
		if err == nil {
			out = append(out, Endpoint{ID: id, Name: friendlyName(dev)})
		}
		dev.release()
	}
	return out, nil
}

// propertyKey is the SDK's PROPERTYKEY.
type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

// propVariant is the SDK's PROPVARIANT on a 64-bit Windows: its type, three
// reserved words, and a union whose first word is a string's pointer when
// the type is VT_LPWSTR.
type propVariant struct {
	vt       uint16
	reserved [3]uint16
	val      *uint16
	pad      uintptr
}

// friendlyName is the name Windows shows for a device, "" where it cannot
// be read.
func friendlyName(dev *comObj) string {
	var store *comObj
	if failed(dev.call(slotOpenPropertyStore, stgmRead, uintptr(unsafe.Pointer(&store)))) {
		return ""
	}
	defer store.release()
	key := propertyKey{fmtid: *guid(pkeyDeviceFriendlyNameFmtID), pid: pkeyDeviceFriendlyNamePID}
	var v propVariant
	if failed(store.call(slotPropertyGetValue, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&v)))) {
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&v))) //nolint:errcheck // nothing to do if clearing fails
	if v.vt != vtLPWSTR || v.val == nil {
		return ""
	}
	return windows.UTF16PtrToString(v.val)
}

// Sessions lists every app's sessions on every active endpoint, the
// system sounds left out.
func (s *System) Sessions() ([]Session, error) {
	var out []Session
	for _, flow := range []Flow{Render, Capture} {
		var coll *comObj
		if hr := s.enum.call(slotEnumAudioEndpoints, uintptr(flow), deviceStateActive, uintptr(unsafe.Pointer(&coll))); failed(hr) {
			return nil, hrError("the audio devices", hr)
		}
		var n uint32
		coll.call(slotCollectionGetCount, uintptr(unsafe.Pointer(&n)))
		for i := range n {
			var dev *comObj
			if failed(coll.call(slotCollectionItem, uintptr(i), uintptr(unsafe.Pointer(&dev)))) {
				continue
			}
			id, err := deviceID(dev)
			if err == nil {
				out = s.endpointSessions(out, dev, id, flow)
			}
			dev.release()
		}
		coll.release()
	}
	return out, nil
}

func (s *System) endpointSessions(out []Session, dev *comObj, id string, flow Flow) []Session {
	var mgr *comObj
	if failed(dev.call(slotActivate, uintptr(unsafe.Pointer(guid(iidIAudioSessionManager2))), clsctxAll, 0, uintptr(unsafe.Pointer(&mgr)))) {
		return out
	}
	defer mgr.release()
	var en *comObj
	if failed(mgr.call(slotGetSessionEnumerator, uintptr(unsafe.Pointer(&en)))) {
		return out
	}
	defer en.release()
	var n int32
	en.call(slotEnumeratorGetCount, uintptr(unsafe.Pointer(&n)))
	for i := range n {
		var ctl *comObj
		if failed(en.call(slotGetSession, uintptr(i), uintptr(unsafe.Pointer(&ctl)))) {
			continue
		}
		var state uint32
		ctl.call(slotGetState, uintptr(unsafe.Pointer(&state)))
		var ctl2 *comObj
		hr := ctl.call(slotQueryInterface, uintptr(unsafe.Pointer(guid(iidIAudioSessionControl2))), uintptr(unsafe.Pointer(&ctl2)))
		ctl.release()
		if failed(hr) {
			continue
		}
		var pid uint32
		system := ctl2.call(slotIsSystemSoundsSession) == sOK
		ctl2.call(slotGetProcessID, uintptr(unsafe.Pointer(&pid)))
		ctl2.release()
		if system || pid == 0 {
			continue
		}
		exe := processImage(pid)
		out = append(out, Session{Endpoint: id, Flow: flow, PID: pid, Exe: exe, App: s.appName(exe), Active: state == audioSessionStateActive})
	}
	return out
}

// processImage is a process's executable, "" where it cannot be read.
func processImage(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// appName is the description an executable's version resource gives, or
// its file name without the extension.
func (s *System) appName(exe string) string {
	if exe == "" {
		return ""
	}
	if name, ok := s.names[exe]; ok {
		return name
	}
	name := fileDescription(exe)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	}
	s.names[exe] = name
	return name
}

func fileDescription(exe string) string {
	size, err := windows.GetFileVersionInfoSize(exe, nil)
	if err != nil || size == 0 {
		return ""
	}
	info := make([]byte, size)
	if windows.GetFileVersionInfo(exe, 0, size, unsafe.Pointer(&info[0])) != nil {
		return ""
	}
	var tr unsafe.Pointer
	var trLen uint32
	if windows.VerQueryValue(unsafe.Pointer(&info[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &trLen) != nil || trLen < 4 {
		return ""
	}
	t := unsafe.Slice((*byte)(tr), 4)
	sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, binary.LittleEndian.Uint16(t), binary.LittleEndian.Uint16(t[2:]))
	var p unsafe.Pointer
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&info[0]), sub, unsafe.Pointer(&p), &n) != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(p)))
}

// waveFormat is WAVEFORMATEX.
type waveFormat struct {
	Tag            uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

const (
	waveFormatPCM        = 1
	waveFormatFloat      = 3
	waveFormatExtensible = 0xFFFE
)

// Stream records one endpoint: its microphone, or what it plays.
type Stream struct {
	dev, client, capture *comObj
	conv                 *converter
	block                int
	started              bool
}

// Record opens a shared-mode stream on the endpoint: a capture endpoint's
// own input, or a render endpoint's output by loopback. It asks the engine
// for 16 kHz mono and, where the engine refuses, takes the engine's own
// format and converts it here.
func (s *System) Record(endpoint string, flow Flow) (*Stream, error) {
	id, err := windows.UTF16PtrFromString(endpoint)
	if err != nil {
		return nil, err
	}
	st := &Stream{}
	if hr := s.enum.call(slotGetDevice, uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(&st.dev))); failed(hr) {
		return nil, hrError("the audio device", hr)
	}
	loopback := flow == Render
	if err := st.open(loopback); err != nil {
		st.Close()
		return nil, err
	}
	if hr := st.client.call(slotStart); failed(hr) {
		st.Close()
		return nil, hrError("starting the recording", hr)
	}
	st.started = true
	return st, nil
}

func (st *Stream) activate() error {
	st.client.release()
	st.client = nil
	if hr := st.dev.call(slotActivate, uintptr(unsafe.Pointer(guid(iidIAudioClient))), clsctxAll, 0, uintptr(unsafe.Pointer(&st.client))); failed(hr) {
		return hrError("the audio device", hr)
	}
	return nil
}

func (st *Stream) open(loopback bool) error {
	if err := st.activate(); err != nil {
		return err
	}
	want := waveFormat{Tag: waveFormatPCM, Channels: 1, SamplesPerSec: Rate, AvgBytesPerSec: Rate * 2, BlockAlign: 2, BitsPerSample: 16}
	mode, flags := initArgs(loopback, true)
	hr := st.client.call(slotInitialize, uintptr(mode), uintptr(flags), hnsBuffer, 0, uintptr(unsafe.Pointer(&want)), 0)
	runtime.KeepAlive(&want)
	if failed(hr) {
		if hr == errDeviceInvalidated || hr == errAccessDenied {
			return hrError("opening the recording", hr)
		}
		// The engine's own format, converted here. A client that failed
		// Initialize cannot be used again.
		if err := st.activate(); err != nil {
			return err
		}
		var mix unsafe.Pointer
		if hr := st.client.call(slotGetMixFormat, uintptr(unsafe.Pointer(&mix))); failed(hr) {
			return hrError("the device's format", hr)
		}
		defer windows.CoTaskMemFree(mix)
		f, ok := readFormat(mix)
		if !ok {
			return fmt.Errorf("the device's format cannot be read (%+v)", f)
		}
		mode, flags := initArgs(loopback, false)
		if hr := st.client.call(slotInitialize, uintptr(mode), uintptr(flags), hnsBuffer, 0, uintptr(mix), 0); failed(hr) {
			return hrError("opening the recording", hr)
		}
		st.conv, st.block = newConverter(f), f.BlockAlign
	} else {
		st.conv, st.block = newConverter(direct), direct.BlockAlign
	}
	if hr := st.client.call(slotGetService, uintptr(unsafe.Pointer(guid(iidIAudioCaptureClient))), uintptr(unsafe.Pointer(&st.capture))); failed(hr) {
		return hrError("the recording", hr)
	}
	return nil
}

// readFormat reads a WAVEFORMATEX, or the WAVEFORMATEXTENSIBLE it begins.
func readFormat(p unsafe.Pointer) (Format, bool) {
	wf := (*waveFormat)(p)
	f := Format{Channels: int(wf.Channels), Rate: int(wf.SamplesPerSec), Bits: int(wf.BitsPerSample), BlockAlign: int(wf.BlockAlign)}
	tag := wf.Tag
	if tag == waveFormatExtensible && wf.Size >= 22 {
		// The sub-format GUID, after the valid bits and the channel mask,
		// begins with the format tag. The samples are read at their
		// container's width, whatever part of it is valid.
		b := unsafe.Slice((*byte)(p), 40)
		tag = binary.LittleEndian.Uint16(b[24:])
	}
	f.Float = tag == waveFormatFloat
	return f, (tag == waveFormatPCM || tag == waveFormatFloat) && f.supported()
}

// Read returns what has arrived since the last Read, as 16 kHz mono
// samples; nil when nothing has. A silent packet is read as zeros.
func (st *Stream) Read() ([]int16, error) {
	var out []int16
	for {
		var n uint32
		if hr := st.capture.call(slotGetNextPacketSize, uintptr(unsafe.Pointer(&n))); failed(hr) {
			return out, hrError("reading the recording", hr)
		}
		if n == 0 {
			return out, nil
		}
		var data unsafe.Pointer
		var frames, flags uint32
		if hr := st.capture.call(slotGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), 0, 0); failed(hr) {
			return out, hrError("reading the recording", hr)
		}
		size := int(frames) * st.block
		b := make([]byte, size)
		if flags&bufferFlagsSilent == 0 && size > 0 {
			copy(b, unsafe.Slice((*byte)(data), size))
		}
		st.capture.call(slotReleaseBuffer, uintptr(frames))
		out = append(out, st.conv.convert(b)...)
	}
}

// Close stops the stream and lets go of the device.
func (st *Stream) Close() {
	if st.started {
		st.client.call(slotStop)
		st.started = false
	}
	st.capture.release()
	st.client.release()
	st.dev.release()
	st.capture, st.client, st.dev = nil, nil, nil
}
