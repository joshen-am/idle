//go:build darwin

package main

import (
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Teclado de todo el sistema en macOS: un "event tap" (CGEventTap) en la sesión. macOS le
// muestra cada tecla antes que a las apps; si devolvemos NULL, la app no la recibe.
// Requiere el permiso de Accesibilidad (Ajustes > Privacidad y seguridad > Accesibilidad).

const (
	rtldNow    = 0x2
	rtldGlobal = 0x8

	kCGSessionEventTap        = 1
	kCGHeadInsertEventTap     = 0
	kCGEventTapOptionDefault  = 0
	kCGEventKeyDown           = 10
	kCGEventKeyUp             = 11
	kCGKeyboardEventKeycode   = 9
	kCGEventTapDisabledByTime = 0xFFFFFFFE
	kCGEventTapDisabledByUser = 0xFFFFFFFF

	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
)

var (
	cgEventTapCreate              func(tap, place, options uint32, mask uint64, callback, refcon uintptr) uintptr
	cgEventTapEnable              func(tap uintptr, enable bool)
	cgEventGetIntegerValueField   func(event uintptr, field uint32) int64
	cgEventGetFlags               func(event uintptr) uint64
	cfMachPortCreateRunLoopSource func(alloc, port uintptr, order int) uintptr
	cfRunLoopGetCurrent           func() uintptr
	cfRunLoopAddSource            func(rl, source, mode uintptr)
	cfRunLoopRun                  func()
	cfDictionaryCreate            func(alloc uintptr, keys, values *uintptr, n int, keyCB, valueCB uintptr) uintptr
	axIsProcessTrusted            func() bool
	axIsProcessTrustedWithOpts    func(options uintptr) bool

	tapPort uintptr
)

// macKey traduce el código de tecla de macOS. ok=false: la tecla pasa siempre (F1-F12...).
func macKey(code int64) (Key, bool) {
	letters := map[int64]Key{0: 'A', 11: 'B', 8: 'C', 2: 'D', 14: 'E', 3: 'F', 5: 'G', 4: 'H', 34: 'I', 38: 'J',
		40: 'K', 37: 'L', 46: 'M', 45: 'N', 31: 'O', 35: 'P', 12: 'Q', 15: 'R', 1: 'S', 17: 'T', 32: 'U', 9: 'V',
		13: 'W', 7: 'X', 16: 'Y', 6: 'Z', 29: '0', 18: '1', 19: '2', 20: '3', 21: '4', 23: '5', 22: '6', 26: '7',
		28: '8', 25: '9', 126: KUp, 125: KDown, 123: KLeft, 124: KRight, 36: KEnter, 76: KEnter, 49: KSpace,
		53: KEsc, 51: KBack, 48: KTab}
	if k, ok := letters[code]; ok {
		return k, true
	}
	switch code {
	case 122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111, 105, 107, 113, 106, 64, 79, 80, 90: // F1-F20
		return 0, false
	}
	return KOther, true
}

func tapCallback(proxy, typ, event, refcon uintptr) uintptr {
	switch uint32(typ) {
	case kCGEventTapDisabledByTime, kCGEventTapDisabledByUser: // macOS lo apaga si tarda: lo prendemos
		if tapPort != 0 {
			cgEventTapEnable(tapPort, true)
		}
		return event
	case kCGEventKeyDown, kCGEventKeyUp:
		k, ok := macKey(cgEventGetIntegerValueField(event, kCGKeyboardEventKeycode))
		if !ok {
			return event
		}
		mod := cgEventGetFlags(event)&(flagControl|flagOption|flagCommand) != 0
		if gin.onKey(k, uint32(typ) == kCGEventKeyDown, mod) {
			return 0 // tragada: la app no la recibe
		}
	}
	return event
}

// symValue lee una constante global de una librería (p. ej. kCFRunLoopCommonModes).
func symValue(lib uintptr, name string) uintptr {
	p, err := purego.Dlsym(lib, name)
	if err != nil || p == 0 {
		return 0
	}
	return **(**uintptr)(unsafe.Pointer(&p))
}

func loadMacLibs() (cf, ax uintptr, err error) {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", rtldNow|rtldGlobal)
	if err != nil {
		return
	}
	cf, err = purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", rtldNow|rtldGlobal)
	if err != nil {
		return
	}
	ax, err = purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", rtldNow|rtldGlobal)
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&cgEventTapCreate, cg, "CGEventTapCreate")
	purego.RegisterLibFunc(&cgEventTapEnable, cg, "CGEventTapEnable")
	purego.RegisterLibFunc(&cgEventGetIntegerValueField, cg, "CGEventGetIntegerValueField")
	purego.RegisterLibFunc(&cgEventGetFlags, cg, "CGEventGetFlags")
	purego.RegisterLibFunc(&cfMachPortCreateRunLoopSource, cf, "CFMachPortCreateRunLoopSource")
	purego.RegisterLibFunc(&cfRunLoopGetCurrent, cf, "CFRunLoopGetCurrent")
	purego.RegisterLibFunc(&cfRunLoopAddSource, cf, "CFRunLoopAddSource")
	purego.RegisterLibFunc(&cfRunLoopRun, cf, "CFRunLoopRun")
	purego.RegisterLibFunc(&cfDictionaryCreate, cf, "CFDictionaryCreate")
	purego.RegisterLibFunc(&axIsProcessTrusted, ax, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axIsProcessTrustedWithOpts, ax, "AXIsProcessTrustedWithOptions")
	return
}

// askAccessibility muestra el aviso de macOS para dar el permiso de Accesibilidad.
func askAccessibility(cf, ax uintptr) {
	key := symValue(ax, "kAXTrustedCheckOptionPrompt")
	val := symValue(cf, "kCFBooleanTrue")
	kcb, _ := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
	vcb, _ := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
	if key == 0 || val == 0 {
		axIsProcessTrusted()
		return
	}
	dict := cfDictionaryCreate(0, &key, &val, 1, kcb, vcb)
	axIsProcessTrustedWithOpts(dict)
}

func startHook() {
	gin.setStatus(HookStarting)
	go func() {
		runtime.LockOSThread() // el event tap vive en este hilo con su propio "run loop"
		cf, ax, err := loadMacLibs()
		if err != nil {
			logf("teclado: no se pudieron cargar las librerías: %v", err)
			gin.setStatus(HookFailed)
			return
		}
		if !axIsProcessTrusted() {
			gin.setStatus(HookPermission)
			logf("teclado: falta el permiso de Accesibilidad; se pide")
			askAccessibility(cf, ax)
			for !axIsProcessTrusted() {
				time.Sleep(2 * time.Second)
			}
			logf("teclado: permiso de Accesibilidad concedido")
		}
		var tap uintptr
		cb := purego.NewCallback(tapCallback) // una sola vez: hay un límite de callbacks
		for i := 0; tap == 0; i++ {
			tap = cgEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionDefault,
				1<<kCGEventKeyDown|1<<kCGEventKeyUp, cb, 0)
			if tap == 0 {
				if i == 0 {
					logf("teclado: macOS no dejó crear el event tap; se reintenta")
				}
				gin.setStatus(HookPermission)
				time.Sleep(2 * time.Second)
			}
		}
		tapPort = tap
		src := cfMachPortCreateRunLoopSource(0, tap, 0)
		cfRunLoopAddSource(cfRunLoopGetCurrent(), src, symValue(cf, "kCFRunLoopCommonModes"))
		cgEventTapEnable(tap, true)
		gin.setStatus(HookOK)
		logf("teclado: event tap instalado")
		cfRunLoopRun()
	}()
}
