//go:build darwin

package main

import (
	"os"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Ayudas de macOS para la mascota: el mouse de todo el sistema y devolver el foco a tu app.

var (
	cgEventSourceButtonState func(state uint32, button uint32) bool
	prevPid                  int32
	sysReady                 bool
)

func sysInit() bool {
	if sysReady {
		return true
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", rtldNow|rtldGlobal)
	if err != nil {
		return false
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", rtldNow|rtldGlobal); err != nil {
		return false
	}
	purego.RegisterLibFunc(&cgEventSourceButtonState, cg, "CGEventSourceButtonState")
	sysReady = true
	return true
}

// globalMouseDown: ¿está apretado el botón izquierdo del mouse, en cualquier parte?
func globalMouseDown() bool {
	if !sysInit() {
		return false
	}
	return cgEventSourceButtonState(0, 0) // sesión combinada, botón izquierdo
}

// trackFocus recuerda cuál era tu app activa (la que no es Idle).
func trackFocus() {
	if !sysInit() {
		return
	}
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	front := ws.Send(objc.RegisterName("frontmostApplication"))
	if front == 0 {
		return
	}
	if pid := objc.Send[int32](front, objc.RegisterName("processIdentifier")); int(pid) != os.Getpid() {
		prevPid = pid
	}
}

// restoreFocus le devuelve el teclado a tu app después de tocar la mascota.
func restoreFocus() {
	if !sysInit() || prevPid == 0 {
		return
	}
	app := objc.ID(objc.GetClass("NSRunningApplication")).Send(
		objc.RegisterName("runningApplicationWithProcessIdentifier:"), prevPid)
	if app != 0 {
		app.Send(objc.RegisterName("activateWithOptions:"), uint(0))
	}
}
