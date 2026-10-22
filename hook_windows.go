//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Teclado de todo el sistema en Windows: un "hook" de teclado de bajo nivel (WH_KEYBOARD_LL).
// Windows le muestra cada tecla antes que a las apps; si devolvemos 1, la app no la recibe.
// No necesita permisos especiales.

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procSetWindowsHookEx = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32.NewProc("CallNextHookEx")
	procGetMessage       = user32.NewProc("GetMessageW")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
)

type kbdllHookStruct struct {
	VkCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}

// winKey traduce el código de tecla de Windows. ok=false: la tecla pasa siempre
// (modificadores, Bloq Mayús, volumen y multimedia, F1-F24, Impr Pant...).
func winKey(vk uint32) (Key, bool) {
	switch {
	case vk == 0x26:
		return KUp, true
	case vk == 0x28:
		return KDown, true
	case vk == 0x25:
		return KLeft, true
	case vk == 0x27:
		return KRight, true
	case vk == 0x0D:
		return KEnter, true
	case vk == 0x20:
		return KSpace, true
	case vk == 0x1B:
		return KEsc, true
	case vk == 0x08:
		return KBack, true
	case vk == 0x09:
		return KTab, true
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return Key(vk), true
	case vk >= 0x10 && vk <= 0x12, vk >= 0xA0 && vk <= 0xA5, vk == 0x5B, vk == 0x5C, vk == 0x5D, vk == 0x14,
		vk >= 0x70 && vk <= 0x87, vk >= 0xA6 && vk <= 0xB7, vk == 0x2C, vk == 0x90, vk == 0x91, vk == 0x13:
		return 0, false
	}
	return KOther, true // escribe o mueve algo en la app (signos, teclado numérico, Supr, Inicio...)
}

func keyHeld(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

func hookProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) == 0 {
		kb := *(**kbdllHookStruct)(unsafe.Pointer(&lParam))
		if k, ok := winKey(kb.VkCode); ok {
			down := wParam == wmKeyDown || wParam == wmSysKeyDown
			mod := keyHeld(0x11) || keyHeld(0x12) || keyHeld(0x5B) || keyHeld(0x5C) // Ctrl, Alt, Windows
			if gin.onKey(k, down, mod) {
				return 1
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return r
}

func startHook() {
	gin.setStatus(HookStarting)
	go func() {
		runtime.LockOSThread() // el hook vive en este hilo, que debe atender mensajes
		mod, _, _ := procGetModuleHandle.Call(0)
		h, _, err := procSetWindowsHookEx.Call(whKeyboardLL, syscall.NewCallback(hookProc), mod, 0)
		if h == 0 {
			logf("teclado: no se pudo instalar el hook: %v", err)
			gin.setStatus(HookFailed)
			return
		}
		gin.setStatus(HookOK)
		logf("teclado: hook instalado")
		var msg [64]byte
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
		}
	}()
}
