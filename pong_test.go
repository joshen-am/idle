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

// En red: si la bola pasa la paleta derecha, se espera el golpe del otro PC antes de dar el punto.
func TestGolpeRemoto(t *testing.T) {
	setup := func() *Pong {
		p := NewPong()
		p.RemoteRight, p.Grace = true, .5
		p.Ph, p.X, p.Y, p.VX, p.VY, p.Sp = PhPlay, W-60, 100, 200, 0, 200
		p.B = 0 // la paleta (según el anfitrión) está lejos de la bola
		return p
	}
	// sin golpe: el punto llega recién después de la espera
	p := setup()
	steps := 0
	for p.Ph == PhPlay && steps < 600 {
		p.Step(Input{}, nil, Dt)
		steps++
	}
	if p.S1 != 1 || float64(steps)*Dt < .5 {
		t.Fatalf("punto %d-%d tras %.2fs", p.S1, p.S2, float64(steps)*Dt)
	}
	// con golpe que llega tarde: la bola vuelve y no hay punto
	p = setup()
	for i := 0; i < 30; i++ {
		p.Step(Input{}, nil, Dt)
	}
	if !p.RemoteHit(100, 90) || p.VX >= 0 || p.S1 != 0 || p.Ev[EvHit2] != 1 {
		t.Fatalf("el golpe remoto no se aplicó: vx %.0f puntos %d ev %d", p.VX, p.S1, p.Ev[EvHit2])
	}
	if p.X > W-PX-PW-BS-1 {
		t.Fatalf("la bola no avanzó tras el golpe: x=%.1f", p.X)
	}
}
