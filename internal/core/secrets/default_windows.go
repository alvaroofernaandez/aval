//go:build windows

package secrets

import (
	"syscall"
	"unsafe"
)

// Default is the Windows Credential Manager, called through advapi32 with no
// PowerShell. List works through the reserved index entry.
func Default() Store { return Indexed(winStore{}) }

var (
	advapi    = syscall.NewLazyDLL("advapi32.dll")
	credRead  = advapi.NewProc("CredReadW")
	credWrite = advapi.NewProc("CredWriteW")
	credDel   = advapi.NewProc("CredDeleteW")
	credFree  = advapi.NewProc("CredFree")
)

const (
	credTypeGeneric = 1
	persistLocal    = 2
	errNotFound     = syscall.Errno(1168)
)

type credential struct {
	Flags, Type        uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist, AttrCount uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type winStore struct{}

func target(name string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(service + "/" + name)
	return p
}

func (winStore) Get(name string) (string, error) {
	var c *credential
	r, _, err := credRead.Call(uintptr(unsafe.Pointer(target(name))), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		if err == errNotFound {
			return "", ErrNotFound
		}
		return "", ErrUnavailable
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	return string(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

func (winStore) Set(name, value string) error {
	b := []byte(value)
	c := credential{Type: credTypeGeneric, TargetName: target(name), CredentialBlobSize: uint32(len(b)), Persist: persistLocal}
	if len(b) > 0 {
		c.CredentialBlob = &b[0]
	}
	if r, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return ErrUnavailable
	}
	return nil
}

func (winStore) Delete(name string) error {
	r, _, err := credDel.Call(uintptr(unsafe.Pointer(target(name))), credTypeGeneric, 0)
	if r == 0 {
		if err == errNotFound {
			return ErrNotFound
		}
		return ErrUnavailable
	}
	return nil
}

func (winStore) List() ([]string, error) { return nil, ErrUnavailable }
