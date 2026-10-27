// Package wasapi is the part of Windows' audio stack that Meeting notes
// uses, called through the COM interfaces' own tables of functions so the
// app needs no C compiler: listing the endpoints, with the names Windows
// shows for them, and the audio sessions on them (which app plays or
// records on which device), and recording from an
// endpoint, a microphone in shared mode or what a speaker or headset plays
// (loopback).
//
// Every identifier and every slot in those tables is written out here and
// held by a test to the Windows SDK's declarations (testdata). A wrong slot
// calls the wrong function, so the package is run only in a child process of
// the app, where a fault ends that process and not the app.
package wasapi

import "strconv"

// The IIDs and the CLSID this package uses, as the SDK declares them.
const (
	clsidMMDeviceEnumerator  = "BCDE0395-E52F-467C-8E3D-C4579291692E"
	iidIMMDeviceEnumerator   = "A95664D2-9614-4F35-A746-DE8DB63617E6"
	iidIAudioClient          = "1CB9AD4C-DBFA-4C32-B178-C2F568A703B2"
	iidIAudioCaptureClient   = "C8ADBD64-E71E-48A0-A4DE-185C395CD317"
	iidIAudioSessionManager2 = "77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F"
	iidIAudioSessionControl2 = "BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D"
	iidIPropertyStore        = "886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99"
)

// pkeyDeviceFriendlyName is the property key of the name Windows shows for
// a device ("Microphone (USB Speakerphone)"): its format id and property id.
const (
	pkeyDeviceFriendlyNameFmtID = "A45C254E-DF1C-4EFD-8020-67D146A850E0"
	pkeyDeviceFriendlyNamePID   = 14
)

// The slots this package calls, each the method's place in its interface's
// table.
const (
	slotQueryInterface = 0
	slotRelease        = 2

	slotEnumAudioEndpoints      = 3 // IMMDeviceEnumerator
	slotGetDefaultAudioEndpoint = 4
	slotGetDevice               = 5

	slotCollectionGetCount = 3 // IMMDeviceCollection
	slotCollectionItem     = 4

	slotActivate          = 3 // IMMDevice
	slotOpenPropertyStore = 4
	slotGetID             = 5

	slotPropertyGetValue = 5 // IPropertyStore

	slotInitialize   = 3 // IAudioClient
	slotGetMixFormat = 8
	slotStart        = 10
	slotStop         = 11
	slotGetService   = 14

	slotGetBuffer         = 3 // IAudioCaptureClient
	slotReleaseBuffer     = 4
	slotGetNextPacketSize = 5

	slotGetSessionEnumerator = 5 // IAudioSessionManager2

	slotEnumeratorGetCount = 3 // IAudioSessionEnumerator
	slotGetSession         = 4

	slotGetState              = 3  // IAudioSessionControl
	slotGetProcessID          = 14 // IAudioSessionControl2
	slotIsSystemSoundsSession = 15
)

// calls names every slot above with its interface and method, and every
// identifier with its interface, for the test that holds them to the SDK.
var calls = []struct {
	Interface, Method string
	Slot              int
}{
	{"IMMDeviceEnumerator", "Release", slotRelease},
	{"IMMDeviceEnumerator", "EnumAudioEndpoints", slotEnumAudioEndpoints},
	{"IMMDeviceEnumerator", "GetDefaultAudioEndpoint", slotGetDefaultAudioEndpoint},
	{"IMMDeviceEnumerator", "GetDevice", slotGetDevice},
	{"IMMDeviceCollection", "GetCount", slotCollectionGetCount},
	{"IMMDeviceCollection", "Item", slotCollectionItem},
	{"IMMDevice", "Activate", slotActivate},
	{"IMMDevice", "OpenPropertyStore", slotOpenPropertyStore},
	{"IMMDevice", "GetId", slotGetID},
	{"IPropertyStore", "Release", slotRelease},
	{"IPropertyStore", "GetValue", slotPropertyGetValue},
	{"IAudioClient", "Initialize", slotInitialize},
	{"IAudioClient", "GetMixFormat", slotGetMixFormat},
	{"IAudioClient", "Start", slotStart},
	{"IAudioClient", "Stop", slotStop},
	{"IAudioClient", "GetService", slotGetService},
	{"IAudioCaptureClient", "GetBuffer", slotGetBuffer},
	{"IAudioCaptureClient", "ReleaseBuffer", slotReleaseBuffer},
	{"IAudioCaptureClient", "GetNextPacketSize", slotGetNextPacketSize},
	{"IAudioSessionManager2", "GetSessionEnumerator", slotGetSessionEnumerator},
	{"IAudioSessionEnumerator", "GetCount", slotEnumeratorGetCount},
	{"IAudioSessionEnumerator", "GetSession", slotGetSession},
	{"IAudioSessionControl", "GetState", slotGetState},
	{"IAudioSessionControl2", "QueryInterface", slotQueryInterface},
	{"IAudioSessionControl2", "GetProcessId", slotGetProcessID},
	{"IAudioSessionControl2", "IsSystemSoundsSession", slotIsSystemSoundsSession},
}

var identifiers = map[string]string{
	"class MMDeviceEnumerator":     clsidMMDeviceEnumerator,
	"IMMDeviceEnumerator":          iidIMMDeviceEnumerator,
	"IAudioClient":                 iidIAudioClient,
	"IAudioCaptureClient":          iidIAudioCaptureClient,
	"IAudioSessionManager2":        iidIAudioSessionManager2,
	"IAudioSessionControl2":        iidIAudioSessionControl2,
	"IPropertyStore":               iidIPropertyStore,
	"key PKEY_Device_FriendlyName": pkeyDeviceFriendlyNameFmtID + " " + strconv.Itoa(pkeyDeviceFriendlyNamePID),
}

// Shared mode, and the stream flags a stream is opened with: the engine
// converts to the format asked for, and a loopback stream records what an
// output endpoint plays. Exclusive mode, which would take the device from
// the meeting app, is never asked for, and no stream is given the
// communications category, which can make Windows turn other sounds down.
const (
	shareModeShared         = 0
	streamFlagsLoopback     = 0x00020000
	streamFlagsNoPersist    = 0x00080000
	streamFlagsSrcDefault   = 0x08000000
	streamFlagsAutoConvert  = 0x80000000
	bufferFlagsSilent       = 0x2
	hnsBuffer               = 10_000_000 // a second, in 100 ns units
	errDeviceInvalidated    = 0x88890004
	errUnsupportedFormat    = 0x88890008
	errAccessDenied         = 0x80070005
	audioSessionStateActive = 1
)

// initArgs are the share mode and stream flags of a stream: shared, with
// the engine's conversion unless the stream takes the engine's own format,
// loopback for an output endpoint. Settings Windows keeps per app are not
// written (NOPERSIST).
func initArgs(loopback, convert bool) (shareMode, flags uint32) {
	flags = streamFlagsNoPersist
	if convert {
		flags |= streamFlagsAutoConvert | streamFlagsSrcDefault
	}
	if loopback {
		flags |= streamFlagsLoopback
	}
	return shareModeShared, flags
}
