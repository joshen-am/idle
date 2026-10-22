package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"time"
)

// Pong de 4 en red. El que llega primero es el anfitrión: abre tres "salas" (una por invitado,
// cada una con su propia conexión, así el servidor no corta por exceso de mensajes) y calcula
// la partida. Cada invitado mueve su paleta, decide sus propios golpes y ve la bola calculada
// desde el último rebote. Los lugares que faltan se llenan con CPU.

type Slot4 struct {
	Kind uint8
	Av   Avatar
	CPU  int // índice en cpus
	Link int // anfitrión: por qué conexión llega (-1 = yo)
	ID   string
}

var sideNames = []string{"ABAJO", "ARRIBA", "IZQUIERDA", "DERECHA"}

// orden en que se llenan los lugares
var fillOrder = []int{SBottom, STop, SLeft, SRight}

type Match4 struct {
	room    string
	me      Avatar
	prof    Profile
	snd     *Sound
	host    bool
	decided bool
	offline bool
	links   [3]*Net
	sub     int // invitado: en qué sala estoy probando
	findAt  time.Time

	Slots   [4]Slot4
	MySide  int
	Started bool
	Full    bool // invitado: todas las salas llenas
	p       *Pong4
	tick    int
	round   uint8

	// invitado
	lastZ     int
	lastY     int
	lat       float64
	corrX     float64
	corrY     float64
	targets   [4]float64
	hitAt     int
	hitMsg    []byte
	hitLocal  float64 // cuándo golpeé (tiempo local)
	clock     float64
	lastEv    [EvCount]uint8
	evReady   bool
	trail     [][2]float64
	flash     [4]float64
	recorded  bool
	remoteHid [3]bool
	ySent     time.Time
}

func NewMatch4(room string, me Avatar, prof Profile, snd *Sound) *Match4 {
	m := &Match4{room: room, me: me, prof: prof, snd: snd, MySide: -1, p: NewPong4(), findAt: time.Now()}
	m.links[0] = AutoRoom(room + "-4a")
	return m
}

func subRoom(room string, k int) string { return room + "-4" + string(rune('a'+k)) }

func (m *Match4) Host() bool { return m.host }

// Status: qué mostrar en la sala de espera.
func (m *Match4) Status() string {
	switch {
	case m.Full:
		return "LA SALA ESTA LLENA"
	case !m.decided:
		return "BUSCANDO" + dots(m.tick)
	case m.offline && m.host:
		return "SIN INTERNET: JUEGA CON CPU"
	case m.host:
		return "ESPERANDO JUGADORES" + dots(m.tick)
	case m.MySide < 0:
		return "PARTIDA EN CURSO, ESPERA" + dots(m.tick)
	}
	return "ESPERANDO QUE EMPIECE" + dots(m.tick)
}

func (m *Match4) Players() int {
	n := 0
	for _, s := range m.Slots {
		if s.Kind != SlotEmpty {
			n++
		}
	}
	return n
}

func (m *Match4) becomeHost() {
	m.host, m.decided, m.MySide = true, true, SBottom
	m.Slots[SBottom] = Slot4{Kind: SlotHuman, Av: m.me, Link: -1, ID: m.prof.ID}
	for k := 1; k < 3; k++ {
		if m.links[k] == nil {
			m.links[k] = AutoRoom(subRoom(m.room, k))
		}
	}
	logf("x4: soy el anfitrión")
}

// AddCPU pone un personaje al azar (de los que no están) en el próximo lugar libre.
func (m *Match4) AddCPU() {
	if !m.host || m.Started {
		return
	}
	used := map[int]bool{}
	for _, s := range m.Slots {
		if s.Kind == SlotCPU {
			used[s.CPU] = true
		}
	}
	var free []int
	for i := range cpus {
		if !used[i] {
			free = append(free, i)
		}
	}
	for _, side := range fillOrder {
		if m.Slots[side].Kind == SlotEmpty && len(free) > 0 {
			c := free[rand.Intn(len(free))]
			m.Slots[side] = Slot4{Kind: SlotCPU, Av: cpus[c].Av, CPU: c, Link: -1, ID: cpus[c].ID}
			return
		}
	}
}

func (m *Match4) RemoveCPU(side int) {
	if m.host && !m.Started && m.Slots[side].Kind == SlotCPU {
		m.Slots[side] = Slot4{}
	}
}

func (m *Match4) CanStart() bool { return m.host && !m.Started && m.Players() >= 2 }

// Start: el anfitrión arranca (o la revancha).
func (m *Match4) Start() {
	if !m.host || m.Players() < 2 {
		return
	}
	p := m.p
	for i, s := range m.Slots {
		p.Kind[i] = s.Kind
		p.Remote[i] = s.Kind == SlotHuman && s.Link >= 0
		p.CPUSp[i], p.CPUErr[i] = 0, 0
		if s.Kind == SlotCPU {
			p.CPUSp[i], p.CPUErr[i] = cpus[s.CPU].Speed, cpus[s.CPU].Err
		}
	}
	p.Start()
	m.Started, m.recorded = true, false
	m.round++
	m.sendLobby(true)
}

// ---------- cada cuadro ----------

// Update: dir = mis flechas a lo largo de mi lado; hidden = estoy en modo trabajo.
func (m *Match4) Update(dir float64, hidden bool) {
	m.tick++
	m.lobby()
	if m.host {
		m.updateHost(dir, hidden)
	} else if m.decided {
		m.updateGuest(dir, hidden)
	}
	m.effects(hidden)
}

// lobby: decidir si soy anfitrión o invitado y ordenar quién juega dónde.
func (m *Match4) lobby() {
	if !m.decided {
		n := m.links[0]
		switch {
		case n.Status() == NetWaiting:
			m.becomeHost()
		case n.Connected() && n.Full():
			n.Close()
			m.sub++
			if m.sub >= 3 {
				m.Full, m.decided = true, true
				return
			}
			m.links[0] = AutoRoom(subRoom(m.room, m.sub))
		case n.Connected():
			if _, c := n.Raw('Y'); c > 0 {
				m.decided = true
				logf("x4: entré como invitado (sala %d)", m.sub)
			}
		case n.Status() == NetNoInternet && time.Since(m.findAt) > 3*time.Second:
			m.offline = true
			m.becomeHost()
		}
		return
	}
	if !m.host {
		return
	}
	if m.offline && m.links[0].Status() == NetWaiting {
		m.offline = false
	}
	for k, n := range m.links {
		if n == nil {
			continue
		}
		side := -1
		for i, s := range m.Slots {
			if s.Kind == SlotHuman && s.Link == k {
				side = i
			}
		}
		switch {
		case n.Connected() && n.Lost():
			logf("x4: se fue el jugador de la sala %d", k)
			n.Close()
			m.links[k] = AutoRoom(subRoom(m.room, k))
			if side >= 0 {
				m.Slots[side] = Slot4{}
				if m.Started {
					m.p.Kind[side] = SlotEmpty
					m.p.Lives[side] = 0
					if m.p.AliveCount() <= 1 && m.p.Ph != PhOver {
						m.p.loseLife(side) // termina la partida
					}
				}
			}
		case n.Connected() && side < 0 && !m.Started:
			for _, sd := range fillOrder { // un lugar libre (o el de un CPU)
				if m.Slots[sd].Kind == SlotEmpty {
					side = sd
					break
				}
			}
			if side < 0 {
				for _, sd := range fillOrder {
					if m.Slots[sd].Kind == SlotCPU {
						side = sd
						break
					}
				}
			}
			if side >= 0 {
				m.Slots[side] = Slot4{Kind: SlotHuman, Av: avAny, Link: k}
				logf("x4: entró un jugador por la sala %d (%s)", k, sideNames[side])
			}
		}
		if side >= 0 {
			if av, ok := n.PeerAvatar(); ok {
				m.Slots[side].Av = av
			}
			if pr, ok := n.PeerProfile(); ok {
				m.Slots[side].ID = pr.ID
			}
		}
		if m.tick%180 == 40+k*20 && n.Connected() {
			n.SendAvatar(m.me)
			n.SendProfile(m.prof)
		}
	}
	gap := 500 * time.Millisecond
	if m.Started {
		gap = time.Second
	}
	if time.Since(m.ySent) > gap {
		m.sendLobby(false)
	}
}

// sendLobby: cuenta a cada invitado quién está en cada lugar y cuál es el suyo.
func (m *Match4) sendLobby(force bool) {
	m.ySent = time.Now()
	for k, n := range m.links {
		if n == nil || !n.Connected() {
			continue
		}
		yours := byte(255)
		for i, s := range m.Slots {
			if s.Kind == SlotHuman && s.Link == k {
				yours = byte(i)
			}
		}
		b := []byte{yours, 0, m.round}
		if m.Started {
			b[1] = 1
		}
		for i, s := range m.Slots {
			av := s.Av.Encode()
			b = append(b, s.Kind, byte(m.p.Lives[i]), byte(len(av)))
			b = append(b, av...)
		}
		n.SendRaw('Y', b)
	}
}

func (m *Match4) updateHost(dir float64, hidden bool) {
	p := m.p
	if !m.Started {
		return
	}
	var dirs [4]float64
	var move [4]bool
	paused := hidden
	direct := true
	for i, s := range m.Slots {
		switch {
		case s.Kind == SlotCPU:
			dirs[i], move[i] = p.CPUDir(i), true
		case s.Kind == SlotHuman && s.Link < 0:
			dirs[i], move[i] = dir, !hidden
		case s.Kind == SlotHuman:
			n := m.links[s.Link]
			if b, _ := n.Raw('W'); len(b) >= 5 {
				target := float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
				p.Pad[i] += (target - p.Pad[i]) * .35 // llega a saltos: se suaviza
				paused = paused || b[4]&1 != 0
			}
			direct = direct && n.Direct()
			for _, k := range n.TakeQueue() {
				if len(k) >= 5 && int(k[0]) == i {
					p.RemoteHit(i, float64(math.Float32frombits(binary.LittleEndian.Uint32(k[1:]))))
				}
			}
		}
	}
	p.Grace = .9
	if direct {
		p.Grace = .3
	}
	p.Paused = paused
	p.Step(dirs, move, Dt)
	m.sendState()
}

const zLen = 2 + 4*7 + 2 + 1 + 16 + 4 + 2 + 4 + EvCount + 2 + 4

func (m *Match4) sendState() {
	p := m.p
	b := make([]byte, 0, zLen)
	f := func(v float64) { b = binary.LittleEndian.AppendUint32(b, math.Float32bits(float32(v))) }
	b = binary.LittleEndian.AppendUint16(b, uint16(m.tick))
	f(p.T)
	f(p.X)
	f(p.Y)
	f(p.AX)
	f(p.AY)
	f(p.AVX)
	f(p.AVY)
	b = binary.LittleEndian.AppendUint16(b, uint16(math.Min((p.T-p.AT)*1000, 65000)))
	b = append(b, p.ASeq)
	for i := 0; i < 4; i++ {
		f(p.Pad[i])
	}
	for i := 0; i < 4; i++ {
		b = append(b, byte(p.Lives[i]))
	}
	b = append(b, byte(p.Ph), byte(int8(p.Winner)))
	f(p.Tm)
	b = append(b, p.Ev[:]...)
	fl := byte(0)
	if p.Paused {
		fl = 1
	}
	b = append(b, fl, byte(int8(p.Pending)))
	f(p.Sp)
	for _, n := range m.links { // por el servidor ~6 por segundo (tiene límite); directo, todos
		if n != nil && n.Connected() && (m.tick%10 == 0 || n.Direct()) {
			n.SendRaw('Z', b)
		}
	}
}

func (m *Match4) updateGuest(dir float64, hidden bool) {
	n := m.links[0]
	p := m.p
	m.clock += Dt
	// quién está dónde
	if b, c := n.Raw('Y'); c != m.lastY && len(b) >= 3 {
		m.lastY = c
		if b[0] < 4 {
			m.MySide = int(b[0])
		} else {
			m.MySide = -1
		}
		started := b[1] == 1
		if started && (!m.Started || b[2] != m.round) {
			m.recorded = false
		}
		m.Started, m.round = started, b[2]
		i := 3
		for sd := 0; sd < 4 && i+3 <= len(b); sd++ {
			kind, l := b[i], int(b[i+2])
			av, _ := DecodeAvatar(b[i+3 : min(len(b), i+3+l)])
			if kind == SlotEmpty {
				av = Avatar{}
			}
			m.Slots[sd] = Slot4{Kind: kind, Av: av}
			p.Kind[sd] = kind
			i += 3 + l
		}
		if m.MySide >= 0 {
			m.Slots[m.MySide].Av = m.me
		}
	}
	if m.tick%180 == 30 {
		n.SendAvatar(m.me)
		n.SendProfile(m.prof)
	}
	p.Predict, p.Mine = true, m.MySide
	if p.OnHit == nil {
		p.OnHit = func(side int, rel float64) {
			m.hitAt, m.hitLocal = m.tick, m.clock
			m.hitMsg = binary.LittleEndian.AppendUint32([]byte{byte(side)}, math.Float32bits(float32(rel)))
			m.snd.Play(EvHit1)
			m.flash[side] = -.5 // destello de golpe (no de vida perdida)
		}
	}
	// estado de la partida
	if b, c := n.Raw('Z'); c != m.lastZ && len(b) >= zLen {
		m.lastZ = c
		g := func(i int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))) }
		T, X, Y := g(2), g(6), g(10)
		ax, ay, avx, avy := g(14), g(18), g(22), g(26)
		age := float64(binary.LittleEndian.Uint16(b[30:])) / 1000
		i := 33
		for sd := 0; sd < 4; sd++ {
			m.targets[sd] = g(i + sd*4)
		}
		i += 16
		for sd := 0; sd < 4; sd++ {
			p.Lives[sd] = int(b[i+sd])
		}
		i += 4
		p.Ph, p.Winner = Phase(b[i]), int(int8(b[i+1]))
		i += 2
		p.Tm = g(i)
		i += 4
		copy(p.Ev[:], b[i:i+EvCount])
		i += EvCount
		p.Paused = b[i]&1 != 0
		p.Pending = int(int8(b[i+1]))
		p.Sp = g(i + 2)
		p.T = T
		if !m.evReady {
			m.lastEv, m.evReady = p.Ev, true
			for sd := 0; sd < 4; sd++ {
				p.Pad[sd] = m.targets[sd]
			}
		}
		lat := .08
		if n.Direct() {
			lat = .01
		}
		if p.Ph != PhPlay {
			p.X, p.Y, p.VX, p.VY, m.corrX, m.corrY = X, Y, 0, 0, 0, 0
		} else {
			tx, ty := AdvanceFrom(ax, ay, avx, avy, age+lat)
			justHit := m.clock-m.hitLocal < .5 && (avx*p.VX+avy*p.VY) < 0 // mi rebote todavía no le llega
			if !justHit {
				if math.Hypot(tx-p.X, ty-p.Y) > 60 || p.VX == 0 && p.VY == 0 {
					p.X, p.Y, m.corrX, m.corrY = tx, ty, 0, 0
				} else {
					m.corrX, m.corrY = tx-p.X, ty-p.Y
				}
				p.VX, p.VY = avx, avy
			}
		}
	}
	for sd := 0; sd < 4; sd++ {
		if sd != m.MySide {
			p.Pad[sd] += (m.targets[sd] - p.Pad[sd]) * .4
		}
	}
	if p.Ph == PhPlay && !p.Paused {
		cx, cy := limitVec(m.corrX*.2, m.corrY*.2, 5)
		m.corrX, m.corrY = m.corrX-cx, m.corrY-cy
		p.X, p.Y = p.X+cx, p.Y+cy
	}
	var dirs [4]float64
	var move [4]bool
	if m.MySide >= 0 && !hidden && !p.Paused {
		dirs[m.MySide], move[m.MySide] = dir, true
	}
	evs := p.Ev
	p.Step(dirs, move, Dt)
	p.Ev = evs // los sonidos los decide el anfitrión
	if d := m.tick - m.hitAt; m.hitMsg != nil && (d == 0 || d == 8 || d == 20 || d == 40 || d%3 == 0 && d < 30 && n.Direct()) {
		n.SendRaw('K', m.hitMsg)
	}
	if m.MySide >= 0 {
		fl := byte(0)
		if hidden {
			fl = 1
		}
		b := binary.LittleEndian.AppendUint32(nil, math.Float32bits(float32(p.Pad[m.MySide])))
		n.SendRaw('W', append(b, fl))
	}
	if m.Started && n.Lost() {
		p.Ph = PhDC
	}
}

// effects: sonidos, estela y destellos.
func (m *Match4) effects(hidden bool) {
	p := m.p
	for i := 0; i < EvCount; i++ {
		if p.Ev[i] != m.lastEv[i] && !hidden {
			if !(i == EvHit1 && !m.host && m.tick-m.hitAt < 30) { // mi golpe ya sonó
				m.snd.Play(i)
			}
			if i == EvPoint {
				m.flash[p.Lost] = 1
			}
		}
	}
	m.lastEv = p.Ev
	if p.Ph == PhPlay && !p.Paused {
		m.trail = append(m.trail, [2]float64{p.X, p.Y})
		if len(m.trail) > 7 {
			m.trail = m.trail[1:]
		}
	} else if !p.Paused {
		m.trail = m.trail[:0]
	}
	for i := range m.flash {
		if m.flash[i] > 0 {
			m.flash[i] = math.Max(0, m.flash[i]-Dt*1.5)
		} else if m.flash[i] < 0 {
			m.flash[i] = math.Min(0, m.flash[i]+Dt*3)
		}
	}
}

// Lost: el anfitrión se fue (invitado).
func (m *Match4) Disconnected() bool { return !m.host && m.decided && m.p.Ph == PhDC }

// Result: si terminó y todavía no se anotó, quién ganó.
func (m *Match4) Result() (over bool, won bool, winner Slot4) {
	if m.p.Ph != PhOver || m.recorded || m.MySide < 0 {
		return false, false, Slot4{}
	}
	m.recorded = true
	w := m.p.Winner
	if w < 0 {
		return true, false, Slot4{}
	}
	return true, w == m.MySide, m.Slots[w]
}

func (m *Match4) Close() {
	for _, n := range m.links {
		if n != nil {
			n.Close()
		}
	}
}
