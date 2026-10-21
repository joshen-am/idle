package main

import (
	"math"
	"math/rand"
)

// Medidas en píxeles lógicos de la pantalla de 240x240.
const (
	W        = 240
	H        = 240
	PW       = 4  // ancho paleta
	PH       = 34 // alto paleta
	BS       = 5  // tamaño bola
	PX       = 10 // distancia de la paleta al borde
	WinScore = 7
	PSpeed   = 175.0
	MaxSpeed = 330.0
	Dt       = 1.0 / 60
)

type Phase uint8

const (
	PhCount Phase = iota // cuenta regresiva inicial
	PhServe              // pausa corta antes de cada saque
	PhPlay
	PhOver
	PhDC // se cortó la conexión
)

// Eventos (sonidos/efectos). Se cuentan para poder mandarlos por red sin perderlos.
const (
	EvHit1 = iota
	EvHit2
	EvWall
	EvTick
	EvServe
	EvPoint
	EvWin
	EvCount
)

type Input struct {
	Dir       float64 // -1 arriba, 1 abajo
	Target    float64 // posición del ratón (centro de la paleta)
	HasTarget bool
}

type Pong struct {
	A, B         float64 // y de la paleta izquierda (1) y derecha (2)
	X, Y, VX, VY float64
	Sp           float64
	S1, S2       int
	Ph           Phase
	Tm           float64
	Serve        float64
	Winner       int
	Paused       bool
	Ev           [EvCount]uint8 // contadores de eventos (solo suben)
	cpuErr       float64
}

func NewPong() *Pong {
	p := &Pong{}
	p.Reset()
	return p
}

func (p *Pong) Reset() {
	ev := p.Ev
	serve := 1.0
	if rand.Intn(2) == 0 {
		serve = -1
	}
	*p = Pong{A: H/2 - PH/2, B: H/2 - PH/2, X: W/2 - BS/2, Y: H/2 - BS/2, Ph: PhCount, Tm: 3, Serve: serve, Ev: ev}
}

func (p *Pong) emit(e int) { p.Ev[e]++ }

func clamp(v, a, b float64) float64 { return math.Max(a, math.Min(b, v)) }

func movePaddle(pos float64, in Input, dt float64) float64 {
	if in.HasTarget {
		m := PSpeed * 1.7 * dt
		pos += clamp(in.Target-PH/2-pos, -m, m)
	} else {
		pos += in.Dir * PSpeed * dt
	}
	return clamp(pos, 0, H-PH)
}

// CPUInput mueve la paleta derecha siguiendo la bola, con algo de error para que se pueda ganar.
func (p *Pong) CPUInput() Input {
	c := p.B + PH/2
	t := float64(H / 2)
	if p.Ph == PhPlay && p.VX > 0 {
		t = p.Y + BS/2 + p.cpuErr
	}
	return Input{Dir: clamp((t-c)/12, -.8, .8)}
}

// Step avanza un tick. Si in2 es nil la paleta derecha no se mueve aquí (la manda el compañero).
func (p *Pong) Step(in1 Input, in2 *Input, dt float64) {
	if p.Paused || p.Ph == PhDC {
		return
	}
	p.A = movePaddle(p.A, in1, dt)
	if in2 != nil {
		p.B = movePaddle(p.B, *in2, dt)
	}

	switch p.Ph {
	case PhCount, PhServe:
		before := math.Ceil(p.Tm)
		p.Tm -= dt
		if p.Ph == PhCount && math.Ceil(p.Tm) != before && p.Tm > 0 {
			p.emit(EvTick)
		}
		if p.Tm <= 0 {
			p.launch()
		}
	case PhPlay:
		n := int(math.Max(1, math.Ceil(p.Sp*dt/2))) // sub-pasos: la bola no atraviesa paletas
		for k := 0; k < n; k++ {
			h := dt / float64(n)
			p.X += p.VX * h
			p.Y += p.VY * h
			if p.Y < 0 {
				p.Y, p.VY = 0, math.Abs(p.VY)
				p.emit(EvWall)
			}
			if p.Y > H-BS {
				p.Y, p.VY = H-BS, -math.Abs(p.VY)
				p.emit(EvWall)
			}
			if p.VX < 0 && p.X <= PX+PW && p.X+BS >= PX && p.Y+BS >= p.A && p.Y <= p.A+PH {
				p.hit(1)
			}
			if p.VX > 0 && p.X+BS >= W-PX-PW && p.X <= W-PX && p.Y+BS >= p.B && p.Y <= p.B+PH {
				p.hit(2)
			}
			if p.X < -BS-6 {
				p.point(2)
				break
			}
			if p.X > W+6 {
				p.point(1)
				break
			}
		}
	}
}

func (p *Pong) launch() {
	ang := rand.Float64()*.8 - .4
	p.Ph = PhPlay
	p.X, p.Y, p.Sp = W/2-BS/2, H/2-BS/2, 125
	p.VX = p.Serve * p.Sp * math.Cos(ang)
	p.VY = p.Sp * math.Sin(ang)
	p.emit(EvServe)
}

func (p *Pong) hit(side int) {
	py, dir := p.A, 1.0
	if side == 2 {
		py, dir = p.B, -1
	}
	rel := clamp((p.Y+BS/2-(py+PH/2))/(PH/2+BS/2), -1, 1) // dónde pegó en la paleta
	ang := rel * 1.0
	p.Sp = math.Min(p.Sp*1.07, MaxSpeed)
	p.VX = dir * p.Sp * math.Cos(ang)
	p.VY = p.Sp * math.Sin(ang)
	if side == 1 {
		p.X = PX + PW
		p.emit(EvHit1)
		p.cpuErr = (rand.Float64() - .5) * PH * 1.15
	} else {
		p.X = W - PX - PW - BS
		p.emit(EvHit2)
	}
}

func (p *Pong) point(side int) {
	score := &p.S1
	p.Serve = 1 // saca hacia quien perdió el punto
	if side == 2 {
		score = &p.S2
		p.Serve = -1
	}
	*score++
	p.VX, p.VY = 0, 0
	p.X, p.Y = W/2-BS/2, H/2-BS/2
	p.emit(EvPoint)
	if *score >= WinScore {
		p.Ph, p.Winner = PhOver, side
		p.emit(EvWin)
	} else {
		p.Ph, p.Tm = PhServe, .9
	}
}
