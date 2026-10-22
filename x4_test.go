package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"
)

// Pong de 4 por internet (servidor público): un anfitrión, dos invitados y un CPU, todos en
// este proceso. Los jugadores son bots que siguen la bola. Solo con MJ_REAL=1.
func TestX4Real(t *testing.T) {
	if os.Getenv("MJ_REAL") == "" {
		t.Skip("solo con MJ_REAL=1")
	}
	runX4(t, 75*time.Second)
}

// Lo mismo con un servidor de prueba local que demora como internet, solo por el servidor.
func TestX4Local(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	url, stop := fakeSignal(70 * time.Millisecond)
	defer stop()
	t.Setenv("MJ_SIGNAL", url)
	t.Setenv("MJ_NOLAN", os.Getenv("MJ_X4LAN"))
	if os.Getenv("MJ_X4LAN") != "" {
		t.Setenv("MJ_NOLAN", "")
	} else {
		t.Setenv("MJ_NOLAN", "1")
	}
	runX4(t, 40*time.Second*time.Duration(1+len(os.Getenv("MJ_X4LONG"))))
}

func runX4(t *testing.T, dur time.Duration) {
	room := fmt.Sprintf("Q%X", rand.Intn(1<<20))
	snd := NewSound()
	mk := func(name string) *Match4 {
		av := randomAvatar()
		av.Name = name
		return NewMatch4(room, av, Profile{ID: name}, snd)
	}
	ms := []*Match4{mk("HOST")}
	tick := func() {
		for _, m := range ms {
			dir := 0.0
			if m.MySide >= 0 && m.p.Ph == PhPlay {
				dir = clamp((m.p.along(m.MySide)-(m.p.Pad[m.MySide]+PL4/2))/8, -1, 1)
			}
			m.Update(dir, false)
		}
		time.Sleep(time.Second / 60)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for !ms[0].Host() {
		tick()
		if time.Now().After(deadline) {
			t.Fatal("no llegó a ser anfitrión")
		}
	}
	ms = append(ms, mk("G1"))
	for ms[1].MySide < 0 || !ms[1].decided {
		tick()
		if time.Now().After(deadline) {
			t.Fatalf("G1 no entró: %s", ms[1].Status())
		}
	}
	ms = append(ms, mk("G2"))
	for ms[2].MySide < 0 || !ms[2].decided {
		tick()
		if time.Now().After(deadline) {
			t.Fatalf("G2 no entró: %s (sala %d)", ms[2].Status(), ms[2].sub)
		}
	}
	h := ms[0]
	t.Logf("lugares: G1=%s G2=%s, jugadores %d", sideNames[ms[1].MySide], sideNames[ms[2].MySide], h.Players())
	h.AddCPU()
	h.Start()
	start := time.Now()
	maxJump := [3]float64{}
	prev := [3][2]float64{}
	logged := 0
	lastLog := time.Now()
	for time.Since(start) < dur && h.p.Ph != PhOver {
		t0 := time.Now()
		tick()
		if dt := time.Since(t0); dt > 300*time.Millisecond {
			t.Logf("un cuadro tardó %v", dt)
		}
		if time.Since(lastLog) > 5*time.Second {
			lastLog = time.Now()
			hp := h.p
			t.Logf("anfitrión: ph=%d bola=(%.0f,%.0f) v=(%.0f,%.0f) pend=%d pausa=%v paletas=%.0f golpes=%d | G1 ph=%d bola=(%.0f,%.0f) | G2 ph=%d bola=(%.0f,%.0f)",
				hp.Ph, hp.X, hp.Y, hp.VX, hp.VY, hp.Pending, hp.Paused, hp.Pad, hp.Hits,
				ms[1].p.Ph, ms[1].p.X, ms[1].p.Y, ms[2].p.Ph, ms[2].p.X, ms[2].p.Y)
		}
		for i, m := range ms {
			if m.p.Ph == PhPlay && !m.p.Paused {
				bx, by := m.BallPos() // lo que se ve
				d := math.Hypot(bx-prev[i][0], by-prev[i][1]) - m.p.Sp*Dt
				if prev[i][0] != 0 && d > 25 && logged < 12 {
					logged++
					t.Logf("salto %d: %.0f px de (%.0f,%.0f) a (%.0f,%.0f) v=(%.0f,%.0f) pend=%d vidas=%v", i, d, prev[i][0], prev[i][1], m.p.X, m.p.Y, m.p.VX, m.p.VY, m.p.Pending, m.p.Lives)
				}
				if prev[i][0] != 0 && d > maxJump[i] && d < 200 {
					maxJump[i] = d
				}
				prev[i] = [2]float64{bx, by}
			} else {
				prev[i] = [2]float64{}
			}
			if m.p.Ph == PhDC {
				t.Fatalf("%d: se cortó", i)
			}
		}
	}
	t.Logf("vidas al final %v (abajo, arriba, izq, der), golpes %d, velocidad %.0f", h.p.Lives, h.p.Hits, h.p.Sp)
	t.Logf("saltos extra de la bola por cuadro: anfitrión %.1f, G1 %.1f, G2 %.1f", maxJump[0], maxJump[1], maxJump[2])
	t.Logf("directo G1=%v G2=%v", ms[1].links[0].Direct(), ms[2].links[0].Direct())
	for _, g := range ms[1:] {
		if lost := Lives4 - h.p.Lives[g.MySide]; lost > 2 {
			t.Errorf("%s perdió %d vidas siendo un bot perfecto: sus golpes no llegan bien", sideNames[g.MySide], lost)
		}
		if !g.Started {
			t.Errorf("el invitado no vio empezar la partida")
		}
	}
	for _, m := range ms {
		m.Close()
	}
}
