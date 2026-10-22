//go:build windows

package main

import (
	"os"
	"testing"
	"time"
	"unsafe"
)

var procSendInput = user32.NewProc("SendInput")

// sendKey simula una tecla a nivel del sistema (como si se apretara en el teclado).
func sendKey(vk uint16, up bool) {
	var in [40]byte // INPUT en 64 bits: tipo (4) + relleno (4) + KEYBDINPUT (32)
	in[0] = 1       // INPUT_KEYBOARD
	*(*uint16)(unsafe.Pointer(&in[8])) = vk
	if up {
		*(*uint32)(unsafe.Pointer(&in[12])) = 2 // KEYEVENTF_KEYUP
	}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in[0])), 40)
}

func TestHookWindows(t *testing.T) {
	if os.Getenv("MJ_HOOKTEST") == "" {
		t.Skip("solo con MJ_HOOKTEST=1")
	}
	startHook()
	for i := 0; gin.Status() != HookOK; i++ {
		if i > 50 {
			t.Fatalf("el hook no arrancó (estado %d)", gin.Status())
		}
		time.Sleep(100 * time.Millisecond)
	}
	gesture := func() {
		for i := 0; i < 2; i++ {
			sendKey(0x26, false)
			sendKey(0x28, false)
			sendKey(0x26, true)
			sendKey(0x28, true)
			time.Sleep(80 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
	}
	gesture()
	if _, game := gin.Peek(); !game {
		t.Fatal("el gesto no activó el modo juego")
	}
	before := gin.swallows
	sendKey('A', false)
	sendKey('A', true)
	time.Sleep(300 * time.Millisecond)
	if gin.swallows-before != 2 {
		t.Fatalf("en modo juego la A debía tragarse (bajar y soltar): %d", gin.swallows-before)
	}
	keys, _ := gin.Frame()
	t.Logf("teclas que recibió el juego: %v", keys)
	gesture()
	if _, game := gin.Peek(); game {
		t.Fatal("el gesto no volvió al modo trabajo")
	}
	before = gin.swallows
	sendKey('A', false)
	sendKey('A', true)
	time.Sleep(300 * time.Millisecond)
	if gin.swallows != before {
		t.Fatal("en modo trabajo la A debía pasar a la app")
	}
	t.Log("hook de Windows: gesto, modo juego y modo trabajo funcionan")
}
