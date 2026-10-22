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
	for time.Since(start) < 75*time.Second && h.p.Ph != PhOver {
		tick()
		for i, m := range ms {
			if m.p.Ph == PhPlay && !m.p.Paused {
				d := math.Hypot(m.p.X-prev[i][0], m.p.Y-prev[i][1]) - m.p.Sp*Dt
				if prev[i][0] != 0 && d > maxJump[i] && d < 200 {
					maxJump[i] = d
				}
				prev[i] = [2]float64{m.p.X, m.p.Y}
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
