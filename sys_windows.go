//go:build windows

package main

import (
	"os"
	"unsafe"
)

// Ayudas de Windows para la mascota: el mouse de todo el sistema y devolver el foco.

var (
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	prevForeground               uintptr
)

// globalMouseDown: ¿está apretado el botón izquierdo del mouse, en cualquier parte?
func globalMouseDown() bool { return keyHeld(0x01) }

// trackFocus recuerda cuál era tu ventana activa (la que no es de Idle).
func trackFocus() {
	h, _, _ := procGetForegroundWindow.Call()
	if h == 0 {
		return
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if int(pid) != os.Getpid() {
		prevForeground = h
	}
}

// restoreFocus le devuelve el teclado a tu ventana después de tocar la mascota.
func restoreFocus() {
	if prevForeground != 0 {
		procSetForegroundWindow.Call(prevForeground)
	}
}
