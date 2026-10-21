package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Prueba de conexión real por internet (servidor público). Solo corre con MJ_REAL=1.
// MJ_ROLE=host|guest|both y MJ_ROOM=código permiten probar entre dos máquinas distintas.
func TestConexionReal(t *testing.T) {
	if os.Getenv("MJ_REAL") == "" {
		t.Skip("solo con MJ_REAL=1")
	}
	role, code := os.Getenv("MJ_ROLE"), strings.ToUpper(os.Getenv("MJ_ROOM"))
	if code != "" {
		code = (code + "XXXX")[:4] // siempre 4 letras
	}
	if role == "" {
		role = "both"
	}
	var host, guest *Net
	if role == "host" || role == "both" {
		if code != "" {
			os.Setenv("MJ_CODE", code)
		}
		host = HostRoom()
		defer host.Close()
		deadline := time.Now().Add(20 * time.Second)
		for host.RTCStatus() != NetOnline {
			if time.Now().After(deadline) {
				t.Fatalf("el anfitrión no llegó al servidor (estado %d)", host.RTCStatus())
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Logf("sala creada: %s", host.Code)
		code = host.Code
	}
	if role == "guest" || role == "both" {
		deadline := time.Now().Add(3 * time.Minute)
		for {
			guest = NewSearcher()
			guest.JoinCode(code)
			start := time.Now()
			for time.Since(start) < 35*time.Second && !guest.Connected() && guest.RTCStatus() != NetNoRoom && guest.RTCStatus() != NetNoInternet {
				guest.SendInput(100, 0, 0)
				time.Sleep(50 * time.Millisecond)
			}
			if guest.Connected() {
				break
			}
			t.Logf("intento fallido: estado %d via %q", guest.RTCStatus(), guest.Via())
			guest.Close()
			if time.Now().After(deadline) {
				t.Fatal("el invitado no pudo conectarse")
			}
			time.Sleep(3 * time.Second)
		}
		defer guest.Close()
		t.Logf("invitado conectado por %q", guest.Via())
	}
	// intercambio de paquetes durante 5 segundos
	p := NewPong()
	end := time.Now().Add(5 * time.Second)
	got := false
	for time.Now().Before(end) {
		if host != nil && host.Connected() {
			host.SendState(p, 0)
			if _, _, ok := host.GuestPaddle(); ok {
				got = true
			}
		}
		if guest != nil {
			guest.SendInput(120, 0, 0)
			if _, ok := guest.State(); ok {
				got = true
			}
		}
		time.Sleep(16 * time.Millisecond)
	}
	if !got {
		t.Fatal("conectados pero no pasaron datos")
	}
	if host != nil {
		t.Logf("anfitrión conectado por %q", host.Via())
	}
}
