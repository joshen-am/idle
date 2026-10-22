package main

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

// Prueba de conexión real por internet (servidor público). Solo corre con MJ_REAL=1.
// Con MJ_ROOM las dos máquinas (dos trabajos de GitHub) entran a la misma sala; sin MJ_ROOM,
// los dos jugadores corren en este mismo proceso.
func TestConexionReal(t *testing.T) {
	if os.Getenv("MJ_REAL") == "" {
		t.Skip("solo con MJ_REAL=1")
	}
	room := os.Getenv("MJ_ROOM")
	var nets []*Net
	if room != "" {
		nets = []*Net{AutoRoom(room)}
	} else {
		room = fmt.Sprintf("T%X", rand.Intn(1<<20))
		a := AutoRoom(room)
		for a.Status() != NetWaiting {
			time.Sleep(100 * time.Millisecond)
		}
		nets = []*Net{a, AutoRoom(room)}
	}
	for _, n := range nets {
		defer n.Close()
	}
	deadline := time.Now().Add(4 * time.Minute)
	p := NewPong()
	for {
		ok := true
		for _, n := range nets {
			if n.IsHost() {
				n.SendState(p, 0)
			} else {
				n.SendInput(100, 0, 0)
			}
			ok = ok && n.Connected()
		}
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no se conectaron (estado %d)", nets[0].Status())
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, n := range nets {
		t.Logf("conectado; anfitrión=%v", n.IsHost())
	}
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		for _, n := range nets {
			if n.IsHost() {
				n.SendState(p, 0)
			} else {
				n.SendInput(100, 0, 0)
			}
		}
		time.Sleep(16 * time.Millisecond)
	}
	for _, n := range nets {
		if n.Lost() {
			t.Fatal("se perdió la conexión")
		}
	}
}
