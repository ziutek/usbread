package main

/*
#include <stdlib.h>
#include <libusb.h>

#cgo pkg-config: libusb-1.0
*/
import "C"

import (
	"errors"
	"runtime"
	"unicode/utf16"
	"unsafe"
)

type USBError int

func (e USBError) Error() string {
	return C.GoString(C.libusb_strerror(C.enum_libusb_error(e)))
}

type USBDev struct {
	Serial string

	d *C.struct_libusb_device
}

func (u *USBDev) unref() {
	if u.d == nil {
		panic("USBDev.unref on uninitialized device")
	}
	C.libusb_unref_device(u.d)
	u.d = nil // Help GC.
}

func (u *USBDev) Close() {
	runtime.SetFinalizer(u, nil)
	u.unref()
}

func getLangId(dh *C.libusb_device_handle) (C.uint16_t, error) {
	var buf [128]C.char
	e := C.libusb_get_string_descriptor(
		dh, 0, 0,
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.int(len(buf)),
	)
	if e < 0 {
		return 0, USBError(e)
	}
	if e < 4 {
		return 0, errors.New("not enough data in USB language IDs descriptor")
	}
	return C.uint16_t(uint(buf[2]) | uint(buf[3])<<8), nil
}

func getStringDescriptor(dh *C.libusb_device_handle, id C.uint8_t, langid C.uint16_t) (string, error) {
	var buf [128]C.char
	e := C.libusb_get_string_descriptor(
		dh, id, C.uint16_t(langid),
		(*C.uchar)(unsafe.Pointer(&buf[0])), C.int(len(buf)),
	)
	if e < 0 {
		return "", USBError(e)
	}
	if e < 2 {
		return "", errors.New("not enough data for USB string descriptor")
	}
	l := C.int(buf[0])
	if l > e {
		return "", errors.New("USB string descriptor is too short")
	}
	b := buf[2:l]
	uni16 := make([]uint16, len(b)/2)
	for i := range uni16 {
		uni16[i] = uint16(b[i*2]) | uint16(b[i*2+1])<<8
	}
	return string(utf16.Decode(uni16)), nil
}

// getStrings updates Manufacturer, Description, Serial strings descriptors
// in unicode form. It doesn't use ftdi_usb_get_strings because libftdi
// converts  unicode strings to ASCII.
func (u *USBDev) getStrings(dev *C.libusb_device, ds *C.struct_libusb_device_descriptor) error {
	var (
		err error
		dh  *C.libusb_device_handle
	)
	if e := C.libusb_open(dev, &dh); e != 0 {
		return USBError(e)
	}
	defer C.libusb_close(dh)
	langid, err := getLangId(dh)
	if err != nil {
		return err
	}
	u.Serial, err = getStringDescriptor(dh, ds.iSerialNumber, langid)
	return err
}

// FindAll search for all USB devices with specified vendor and  product id.
// It returns slice od found devices.
func FindAll(vendor, product int) ([]*USBDev, error) {
	if e := C.libusb_init(nil); e != 0 {
		return nil, USBError(e)
	}
	var dptr **C.struct_libusb_device
	if e := C.libusb_get_device_list(nil, &dptr); e < 0 {
		return nil, USBError(e)
	}
	defer C.libusb_free_device_list(dptr, 1)
	devs := (*[1 << 28]*C.libusb_device)(unsafe.Pointer(dptr))

	var n int
	for i, dev := range devs[:] {
		if dev == nil {
			n = i
			break
		}
	}
	descr := make([]*C.struct_libusb_device_descriptor, n)
	for i, dev := range devs[:n] {
		var ds C.struct_libusb_device_descriptor
		if e := C.libusb_get_device_descriptor(dev, &ds); e < 0 {
			return nil, USBError(e)
		}
		if int(ds.idVendor) == vendor && int(ds.idProduct) == product {
			descr[i] = &ds
			continue
		}
		if vendor == 0 && product == 0 && ds.idVendor == 0x403 {
			switch ds.idProduct {
			case 0x6001, 0x6010, 0x6011, 0x6014, 0x6015:
				descr[i] = &ds
				continue
			}
		}
		n--
	}
	if n == 0 {
		return nil, nil
	}
	ret := make([]*USBDev, n)
	n = 0
	for i, ds := range descr {
		if ds == nil {
			continue
		}
		u := new(USBDev)
		u.d = devs[i]
		C.libusb_ref_device(u.d)
		runtime.SetFinalizer(u, (*USBDev).unref)
		if err := u.getStrings(u.d, ds); err != nil {
			return nil, err
		}
		ret[n] = u
		n++
	}
	return ret, nil
}

type USBDH C.libusb_device_handle

func (u *USBDev) Open() (*USBDH, error) {
	var h *C.libusb_device_handle
	if e := C.libusb_open(u.d, &h); e != 0 {
		return nil, USBError(e)
	}
	return (*USBDH)(h), nil
}

func (h *USBDH) C() *C.libusb_device_handle {
	return (*C.libusb_device_handle)(h)
}

func (h *USBDH) Read(ep int, buf []byte) (int, error) {
	var n C.int
	p := (*C.uchar)(unsafe.Pointer(&buf[0]))
	e := C.libusb_bulk_transfer(h.C(), C.uchar(ep), p, C.int(len(buf)), &n, 0)
	if e != 0 {
		return int(n), USBError(e)
	}
	return int(n), nil
}
