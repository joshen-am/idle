package main

import (
	"math"
)

// Una partida de Pong (contra la CPU o en red), sin pantalla ni teclado: el overlay le pasa
// la entrada de cada cuadro y la dibuja. La red funciona como en la versión de ventanita:
// el anfitrión calcula la partida, el invitado decide sus propios golpes y la bola se calcula
// desde el último golpe o saque (ver net.go).

type Mode int

const (
	ModeCPU Mode = iota
	ModeHost
	ModeGuest
)

type Match struct {
	mode Mode
	p    *Pong
	nw   *Net
	my   Avatar
	snd  *Sound
	tick int

	lastEv      [EvCount]uint8
	evReady     bool
	rematch     uint8
	lastRematch uint8
	lastSeq     uint16
	targetA     float64
	targetB     float64 // paleta del invitado, suavizada (anfitrión)
	hitPending  bool    // invitado: golpeé y espero que el anfitrión lo confirme
	hitMsg      HitPkt
	hitAt       int
	skipHit2    uint8
	lat         float64 // demora estimada de ida (invitado)
	corrX       float64 // corrección pendiente de la bola (invitado)
	corrY       float64
	dispX       float64 // bola mostrada (anfitrión)
	dispY       float64
	trail       [][2]float64
	pulse1      float64
	pulse2      float64
}

func NewMatch(mode Mode, nw *Net, my Avatar, snd *Sound) *Match {
	return &Match{mode: mode, p: NewPong(), nw: nw, my: my, snd: snd, evReady: mode != ModeGuest}
}

func (m *Match) Online() bool { return m.mode != ModeCPU }

// Avatars: personaje de la izquierda y de la derecha.
func (m *Match) Avatars() (Avatar, Avatar) {
	peer := avAny
	if m.nw != nil {
		if av, ok := m.nw.PeerAvatar(); ok {
			peer = av
		}
	}
	switch m.mode {
	case ModeCPU:
		return m.my, avCPU
	case ModeHost:
		return m.my, peer
	}
	return peer, m.my
}

// MySide: 1 si juego a la izquierda, 2 a la derecha.
func (m *Match) MySide() int {
	if m.mode == ModeGuest {
		return 2
	}
	return 1
}

// Update avanza un cuadro. in = mi paleta; act = Enter/Espacio (revancha);
// hidden = estoy en modo trabajo (la partida se pausa, también para el otro).
// Devuelve true si la partida terminó por corte de conexión y el jugador pidió salir.
func (m *Match) Update(in Input, act, hidden bool) bool {
	m.tick++
	p := m.p
	if act {
		switch {
		case p.Ph == PhDC:
			return true
		case p.Ph == PhOver && m.mode == ModeGuest:
			m.rematch++
		case p.Ph == PhOver:
			p.Reset()
		}
	}
	var panicFlag uint8
	if hidden {
		panicFlag = 1
		in = Input{}
	}

	switch m.mode {
	case ModeCPU:
		p.Paused = hidden
		in2 := p.CPUInput()
		p.Step(in, &in2, Dt)
	case ModeHost:
		if b, rm, ok := m.nw.GuestPaddle(); ok {
			m.targetB = clamp(b, 0, H-PH)
			p.B += (m.targetB - p.B) * .35 // llega a saltos por el servidor: se suaviza
			if rm != m.lastRematch {
				m.lastRematch = rm
				if p.Ph == PhOver {
					p.Reset()
				}
			}
		}
		if m.nw.Lost() {
			p.Ph = PhDC
		}
		p.Paused = hidden || m.nw.GuestPanic() // si alguno vuelve al trabajo, se pausa para los dos
		if p.Paused {
			panicFlag = 1
		}
		p.RemoteRight, p.Grace = true, .9 // espera el aviso de golpe del otro (por el servidor tarda)
		if m.nw.Direct() {
			p.Grace = .3
		}
		if h, ok := m.nw.TakeHit(); ok && h.Idx == p.Ev[EvHit2] && int(h.Sum) == p.S1+p.S2 {
			p.RemoteHit(h.Y, h.B)
		}
		p.Step(in, nil, Dt)
		m.nw.SendState(p, panicFlag)
	case ModeGuest:
		m.updateGuest(in, panicFlag)
	}
	if m.Online() && m.tick%180 == 30 {
		m.nw.SendAvatar(m.my)
	}

	// sonidos y efectos de lo que pasó
	for i := 0; i < EvCount; i++ {
		if i == EvHit2 && m.mode == ModeGuest && p.Ev[i] == m.skipHit2 && p.Ev[i] != m.lastEv[i] {
			continue // ya sonó cuando golpeé
		}
		if p.Ev[i] != m.lastEv[i] && !hidden {
			m.snd.Play(i)
			switch i {
			case EvHit1:
				m.pulse1 = 1
			case EvHit2:
				m.pulse2 = 1
			}
		}
	}
	m.lastEv = p.Ev

	// en el anfitrión la bola puede saltar cuando llega tarde un golpe del otro: se suaviza
	if d := math.Hypot(p.X-m.dispX, p.Y-m.dispY); m.mode == ModeHost && p.Ph == PhPlay && d > 7 && d < 200 {
		mx, my := limitVec(p.X-m.dispX, p.Y-m.dispY, 11) // como mucho el doble de rápido
		m.dispX, m.dispY = m.dispX+mx, m.dispY+my
	} else {
		m.dispX, m.dispY = p.X, p.Y
	}
	if p.Ph == PhPlay && !p.Paused {
		bx, by := m.BallPos()
		m.trail = append(m.trail, [2]float64{bx, by})
		if len(m.trail) > 6 {
			m.trail = m.trail[1:]
		}
	} else if !p.Paused {
		m.trail = m.trail[:0]
	}
	m.pulse1 = math.Max(0, m.pulse1-Dt*4)
	m.pulse2 = math.Max(0, m.pulse2-Dt*4)
	return false
}

func (m *Match) updateGuest(in Input, panicFlag uint8) {
	p := m.p
	if st, ok := m.nw.State(); ok {
		if m.hitPending && (st.Ev[EvHit2] != m.hitMsg.Idx || int(st.S1+st.S2) != int(m.hitMsg.Sum) || m.tick-m.hitAt > 90) {
			m.hitPending = false // el anfitrión ya lo aplicó (o ya no sirve)
		}
		if st.Seq != m.lastSeq || !m.evReady { // paquete nuevo del anfitrión
			m.lastSeq = st.Seq
			if r := float64(NowMs()-st.Echo-st.Hold) / 1000; r < 2 { // demora de ida y vuelta
				if m.lat == 0 {
					m.lat = r / 2
				}
				m.lat = m.lat*.8 + r/2*.2
			}
			if st.Ph != PhPlay {
				p.X, p.Y, m.corrX, m.corrY = float64(st.X), float64(st.Y), 0, 0
			} else if !m.hitPending { // mientras espero confirmación, sigo mostrando mi rebote
				// la bola se calcula desde el último golpe o saque, adelantada lo que tardó el paquete
				age := float64(st.AAge)/1000 + m.lat
				tx, ty, vx, vy := advanceBall(float64(st.AX), float64(st.AY), float64(st.AVX), float64(st.AVY), age)
				if p.Ph != PhPlay || math.Hypot(tx-p.X, ty-p.Y) > 170 {
					p.X, p.Y, m.corrX, m.corrY = tx, ty, 0, 0
				} else {
					m.corrX, m.corrY = tx-p.X, ty-p.Y
				}
				p.VX, p.VY = vx, vy
				p.Sp = math.Hypot(vx, vy) // para que mis rebotes predichos salgan a la misma velocidad
			}
			p.Tm = float64(st.Tm)
			p.S1, p.S2, p.Ph, p.Winner, p.Ev = int(st.S1), int(st.S2), st.Ph, int(st.Winner), st.Ev
			p.Paused = st.Flags&1 != 0
			m.targetA = float64(st.A)
			if !m.evReady {
				m.lastEv, m.evReady, p.A = p.Ev, true, m.targetA
			}
		}
		if p.Ph == PhPlay && !p.Paused { // entre paquetes: la bola sigue su camino
			cx, cy := limitVec(m.corrX*.2, m.corrY*.2, 6) // corrección repartida, sin saltos
			m.corrX, m.corrY = m.corrX-cx, m.corrY-cy
			p.X, p.Y, p.VX, p.VY = advanceBall(p.X+cx, p.Y+cy, p.VX, p.VY, Dt)
			// predigo el golpe del otro con su paleta: casi siempre acierta y la bola no espera
			if p.VX < 0 && p.X <= PX+PW && p.X+BS >= PX && p.Y+BS >= p.A && p.Y <= p.A+PH {
				saved, err := p.Ev, p.cpuErr
				p.hit(1)
				p.Ev, p.cpuErr = saved, err
			}
		}
		// mis golpes los decido yo, con lo que veo en mi pantalla
		if !m.hitPending && p.Ph == PhPlay && !p.Paused && p.VX > 0 &&
			p.X+BS >= W-PX-PW && p.X <= W-PX && p.Y+BS >= p.B && p.Y <= p.B+PH {
			m.hitPending, m.hitAt = true, m.tick
			m.hitMsg = HitPkt{Idx: p.Ev[EvHit2], Sum: uint8(p.S1 + p.S2), Y: p.Y, B: p.B}
			saved := p.Ev
			p.hit(2) // rebote inmediato en mi pantalla
			p.Ev = saved
			m.snd.Play(EvHit2)
			m.pulse2 = 1
			m.skipHit2 = m.hitMsg.Idx + 1 // que no suene de nuevo al confirmarse
		}
		if d := m.tick - m.hitAt; m.hitPending && (d == 0 || d == 10 || d == 25 || d == 45 || d%3 == 0 && m.nw.Direct()) {
			m.nw.SendHit(m.hitMsg) // pocas veces: el servidor tiene límite de mensajes
		}
		p.A += (m.targetA - p.A) * .5
	}
	if m.nw.Lost() {
		p.Ph = PhDC
	}
	if p.Ph != PhDC {
		if !p.Paused {
			p.B = movePaddle(p.B, in, Dt) // mi paleta la muevo yo: se siente instantánea
		}
		m.nw.SendInput(p.B, m.rematch, panicFlag)
	}
}

// BallPos: dónde dibujar la bola.
func (m *Match) BallPos() (float64, float64) {
	if m.mode == ModeHost { // mientras espera el golpe del otro, la bola queda en su paleta
		return math.Min(m.dispX, W-PX-PW-BS+2), m.dispY
	}
	return m.p.X, m.p.Y
}

func (m *Match) Close() {
	if m.nw != nil {
		m.nw.Close()
	}
}
