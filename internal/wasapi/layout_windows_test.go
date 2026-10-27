package wasapi

import (
	"testing"
	"unsafe"
)

// The structures passed to IPropertyStore::GetValue have the SDK's sizes on
// a 64-bit Windows: PROPERTYKEY is a GUID and a DWORD, PROPVARIANT its type,
// three reserved words and a 16-byte union.
func TestThePropertyStructuresHaveTheSDKsLayout(t *testing.T) {
	if n := unsafe.Sizeof(propertyKey{}); n != 20 {
		t.Errorf("PROPERTYKEY is %d bytes here, 20 in the SDK", n)
	}
	if n := unsafe.Sizeof(propVariant{}); n != 24 {
		t.Errorf("PROPVARIANT is %d bytes here, 24 in the SDK", n)
	}
	if off := unsafe.Offsetof(propVariant{}.val); off != 8 {
		t.Errorf("PROPVARIANT's union is at %d, 8 in the SDK", off)
	}
}
