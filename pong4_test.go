package main

import (
	"math"
	"testing"
)

func TestPong4CPUs(t *testing.T) {
	wins := map[int]int{}
	total := 0.0
	for g := 0; g < 60; g++ {
		p := NewPong4()
		for i := 0; i < 4; i++ {
			p.Kind[i] = SlotCPU
			p.CPUSp[i], p.CPUErr[i] = cpus[i].Speed, cpus[i].Err
		}
		if g%3 == 0 { // también con un lado vacío
			p.Kind[STop] = SlotEmpty
		}
		p.Start()
		maxSp := 0.0
		for i := 0; i < 60*900 && p.Ph != PhOver; i++ {
			var dirs [4]float64
			for s := 0; s < 4; s++ {
				dirs[s] = p.CPUDir(s)
			}
			p.Step(dirs, [4]bool{true, true, true, true}, Dt)
			if p.X < -20 || p.X > F4+20 || p.Y < -20 || p.Y > F4+20 {
				t.Fatalf("la bola se escapó: %.0f,%.0f", p.X, p.Y)
			}
			maxSp = math.Max(maxSp, p.Sp)
		}
		if p.Ph != PhOver {
			t.Fatalf("partida %d no terminó (vidas %v, sp %.0f)", g, p.Lives, p.Sp)
		}
		wins[p.Winner]++
		total += p.T
		if g == 0 {
			t.Logf("duró %.0fs, golpes %d, velocidad máxima %.0f", p.T, p.Hits, maxSp)
		}
	}
	t.Logf("ganadores por lado %v, duración media %.0fs", wins, total/60)
}

// Un jugador remoto: el anfitrión espera su aviso antes de quitarle una vida.
func TestPong4Remoto(t *testing.T) {
	p := NewPong4()
	p.Kind = [4]uint8{SlotHuman, SlotHuman, SlotEmpty, SlotEmpty}
	p.Remote[STop], p.Grace = true, .5
	p.Start()
	p.Ph = PhPlay
	p.X, p.Y, p.VX, p.VY, p.Sp = 140, 60, 0, -200, 200
	p.Pad[STop] = 0 // en el anfitrión parece que no la tapa
	for i := 0; i < 30; i++ {
		p.Step([4]float64{}, [4]bool{}, Dt)
	}
	if p.Pending != STop || p.Lives[STop] != Lives4 {
		t.Fatalf("debería esperar el aviso: pending %d vidas %d", p.Pending, p.Lives[STop])
	}
	if !p.RemoteHit(STop, .2) || p.VY <= 0 {
		t.Fatalf("el aviso no se aplicó")
	}
	// ahora sin aviso: pierde la vida después de la espera
	p.X, p.Y, p.VX, p.VY = 140, 60, 0, -200
	for i := 0; i < 80; i++ {
		p.Step([4]float64{}, [4]bool{}, Dt)
	}
	if p.Lives[STop] != Lives4-1 {
		t.Fatalf("debería perder una vida: %d", p.Lives[STop])
	}
}

// Partida completa sin red: yo (bot) + 3 CPU, hasta que termine, y el resultado se anota una vez.
func TestMatch4SinRed(t *testing.T) {
	m := &Match4{me: Avatar{Name: "YO"}, snd: &Sound{}, MySide: -1, p: NewPong4()}
	m.becomeHost()
	for k := range m.links {
		if m.links[k] != nil {
			m.links[k].Close()
			m.links[k] = nil
		}
	}
	for m.Players() < 4 {
		m.AddCPU()
	}
	m.Start()
	for i := 0; i < 60*400 && m.p.Ph != PhOver; i++ {
		dir := 0.0
		if m.p.Ph == PhPlay {
			dir = clamp((m.p.along(SBottom)-(m.p.Pad[SBottom]+PL4/2))/8, -1, 1)
		}
		m.Update(dir, false)
	}
	over, won, w := m.Result()
	if !over {
		t.Fatalf("no terminó: vidas %v", m.p.Lives)
	}
	t.Logf("duró %.0fs, gané=%v, ganador %s", m.p.T, won, w.Av.DisplayName())
	if again, _, _ := m.Result(); again {
		t.Fatal("el resultado se anotó dos veces")
	}
	m.Start() // revancha
	if m.p.Ph != PhCount || m.p.Lives[SLeft] != Lives4 {
		t.Fatal("la revancha no reinició")
	}
}
