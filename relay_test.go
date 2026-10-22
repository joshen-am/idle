package main

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

// Partida larga simulada por el servidor (modo relevo), al ritmo real del juego.
// Mide los huecos sin recibir datos. Solo con MJ_LONG=1.
func TestRelayLargo(t *testing.T) {
	if os.Getenv("MJ_LONG") == "" {
		t.Skip("solo con MJ_LONG=1")
	}
	room := fmt.Sprintf("L%X", rand.Intn(1<<20))
	host := AutoRoom(room)
	defer host.Close()
	for host.Status() != NetWaiting {
		time.Sleep(100 * time.Millisecond)
	}
	guest := AutoRoom(room)
	defer guest.Close()
	p := NewPong()
	av := randomAvatar()
	start := time.Now()
	var lastH, lastG time.Time
	var maxH, maxG time.Duration
	var lastSeq uint16
	tick := 0
	for time.Since(start) < 3*time.Minute {
		tick++
		if host.Connected() {
			host.SendState(p, 0)
		}
		guest.SendInput(float64(tick%200), 0, 0)
		if tick%180 == 0 {
			host.SendAvatar(av)
			guest.SendAvatar(av)
		}
		now := time.Now()
		if _, _, ok := host.GuestPaddle(); ok {
			host.mu.Lock()
			rx := host.lastRx
			host.mu.Unlock()
			if !lastH.IsZero() && rx.Sub(lastH) > maxH {
				maxH = rx.Sub(lastH)
			}
			lastH = rx
		}
		if st, ok := guest.State(); ok && st.Seq != lastSeq {
			if !lastG.IsZero() && now.Sub(lastG) > maxG {
				maxG = now.Sub(lastG)
				t.Logf("%5.1fs  hueco invitado %v", time.Since(start).Seconds(), maxG)
			}
			lastSeq, lastG = st.Seq, now
		}
		if host.Lost() || guest.Lost() {
			t.Logf("%5.1fs  PERDIDA (anfitrión %v, invitado %v)", time.Since(start).Seconds(), host.Lost(), guest.Lost())
			break
		}
		time.Sleep(time.Second / 60)
	}
	t.Logf("duró %.0fs; hueco máx anfitrión %v, invitado %v", time.Since(start).Seconds(), maxH, maxG)
}
