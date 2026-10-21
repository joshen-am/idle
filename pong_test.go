package main

import "testing"

// Dos CPUs juegan una partida completa: tiene que terminar con un ganador a 7.
func TestPartidaCompleta(t *testing.T) {
	p := NewPong()
	for i := 0; i < 60*60*20 && p.Ph != PhOver; i++ {
		in1 := Input{Dir: clamp((p.Y-(p.A+PH/2))/12, -.8, .8)}
		in2 := p.CPUInput()
		p.Step(in1, &in2, Dt)
	}
	if p.Ph != PhOver || (p.S1 != WinScore && p.S2 != WinScore) {
		t.Fatalf("la partida no terminó: %d-%d fase %d", p.S1, p.S2, p.Ph)
	}
	if p.Ev[EvWin] != 1 || int(p.Ev[EvPoint]) != p.S1+p.S2 {
		t.Fatalf("eventos raros: %+v", p.Ev)
	}
	t.Logf("resultado %d-%d, golpes %d/%d", p.S1, p.S2, p.Ev[EvHit1], p.Ev[EvHit2])
}
