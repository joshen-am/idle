package main

import (
	"math"
	"math/rand"
)

// Pong de 4: una cancha cuadrada con una paleta por lado (abajo, arriba, izquierda, derecha).
// Cada uno defiende su lado; si la bola se le pasa pierde una vida. Sin vidas (o sin jugador),
// ese lado es una pared. Gana el último que queda. La bola acelera con cada golpe y cada saque
// sale más rápido que el anterior.

const (
	F4      = 300.0 // lado de la cancha
	M4      = 12.0  // distancia del borde a la paleta
	PW4     = 5.0   // grosor de la paleta
	PL4     = 46.0  // largo de la paleta
	BS4     = 6.0   // bola
	Corner4 = 30.0  // esquinas: siempre pared
	Lives4  = 4
	PSpd4   = 270.0
	MaxSp4  = 560.0
)

// Lados
const (
	SBottom = iota
	STop
	SLeft
	SRight
)

const (
	SlotEmpty = iota
	SlotHuman
	SlotCPU
)

type Pong4 struct {
	Pad    [4]float64 // posición a lo largo del lado (borde izquierdo o de arriba de la paleta)
	Kind   [4]uint8
	Lives  [4]int
	CPUSp  [4]float64
	CPUErr [4]float64
	Remote [4]bool // la paleta la mueve otro PC (anfitrión: espera su aviso de golpe)
	Grace  float64

	X, Y, VX, VY, Sp float64
	Ph               Phase
	Tm               float64
	T                float64
	Winner           int
	Serves           int
	Hits             uint16
	Ev               [EvCount]uint8
	Lost             int // el último que perdió una vida
	Paused           bool

	// anfitrión: la bola llegó a la línea de un jugador remoto y se espera su aviso
	Pending int
	pendAt  float64

	// desde el último cambio de dirección la bola va en línea recta (para el invitado)
	AX, AY, AVX, AVY, AT float64
	ASeq                 uint8

	cpuErr [4]float64
	// Predict: el invitado calcula la bola para verla fluida, pero no anota puntos
	Predict bool
	Mine    int // invitado: mi lado (mis golpes los decido yo)
	OnHit   func(side int, rel float64)
	Reached [4]int // cuántas veces llegó la bola a cada paleta (pruebas)
}

func NewPong4() *Pong4 {
	p := &Pong4{Winner: -1, Pending: -1, Mine: -1}
	for i := range p.Pad {
		p.Pad[i] = (F4 - PL4) / 2
	}
	return p
}

func (p *Pong4) Alive(i int) bool { return p.Kind[i] != SlotEmpty && p.Lives[i] > 0 }

func (p *Pong4) AliveCount() int {
	n := 0
	for i := 0; i < 4; i++ {
		if p.Alive(i) {
			n++
		}
	}
	return n
}

// Start: empieza (o vuelve a empezar) con los jugadores puestos en Kind.
func (p *Pong4) Start() {
	for i := 0; i < 4; i++ {
		p.Lives[i] = 0
		if p.Kind[i] != SlotEmpty {
			p.Lives[i] = Lives4
		}
		p.Pad[i] = (F4 - PL4) / 2
	}
	p.Ph, p.Tm, p.T, p.Winner, p.Serves, p.Pending = PhCount, 3, 0, -1, 0, -1
	p.X, p.Y, p.VX, p.VY = F4/2-BS4/2, F4/2-BS4/2, 0, 0
	p.emit(EvTick)
	p.anchor()
}

func (p *Pong4) emit(e int) { p.Ev[e]++ }

func (p *Pong4) anchor() {
	p.AX, p.AY, p.AVX, p.AVY, p.AT = p.X, p.Y, p.VX, p.VY, p.T
	p.ASeq++
}

// serve: la bola sale del centro hacia un jugador vivo, cada saque un poco más rápido.
func (p *Pong4) serve() {
	var alive []int
	for i := 0; i < 4; i++ {
		if p.Alive(i) {
			alive = append(alive, i)
		}
	}
	target := alive[rand.Intn(len(alive))]
	p.Serves++
	p.Sp = math.Min(170+15*float64(p.Serves), 360)
	a := (rand.Float64() - .5) * 1.0
	nx, ny := sideNormal(target) // hacia afuera de la cancha por ese lado
	tx, ty := -ny, nx
	p.X, p.Y = F4/2-BS4/2, F4/2-BS4/2
	p.VX = (nx*math.Cos(a) + tx*math.Sin(a)) * p.Sp
	p.VY = (ny*math.Cos(a) + ty*math.Sin(a)) * p.Sp
	p.Ph = PhPlay
	p.emit(EvServe)
	p.newErr()
	p.anchor()
}

// sideNormal: hacia dónde queda ese lado (vector unitario saliendo de la cancha).
func sideNormal(s int) (float64, float64) {
	switch s {
	case SBottom:
		return 0, 1
	case STop:
		return 0, -1
	case SLeft:
		return -1, 0
	}
	return 1, 0
}

// lineOf: la coordenada de la cara de la paleta (donde rebota la bola) de ese lado.
func lineOf(s int) float64 {
	switch s {
	case SBottom, SRight:
		return F4 - M4 - PW4
	}
	return M4 + PW4
}

// along: la coordenada de la bola a lo largo de ese lado (centro de la bola).
func (p *Pong4) along(s int) float64 {
	if s == SBottom || s == STop {
		return p.X + BS4/2
	}
	return p.Y + BS4/2
}

// reach: ¿la bola llegó a la línea de ese lado (yendo hacia él)?
func (p *Pong4) reach(s int) bool {
	switch s {
	case SBottom:
		return p.VY > 0 && p.Y+BS4 >= lineOf(s)
	case STop:
		return p.VY < 0 && p.Y <= lineOf(s)
	case SLeft:
		return p.VX < 0 && p.X <= lineOf(s)
	}
	return p.VX > 0 && p.X+BS4 >= lineOf(s)
}

// out: la bola ya salió de la cancha por ese lado.
func (p *Pong4) out(s int) bool {
	switch s {
	case SBottom:
		return p.Y > F4
	case STop:
		return p.Y+BS4 < 0
	case SLeft:
		return p.X+BS4 < 0
	}
	return p.X > F4
}

// Covers: ¿la paleta de ese lado tapa la bola?
func (p *Pong4) Covers(s int) bool {
	c := p.along(s)
	return c >= p.Pad[s]-BS4/2 && c <= p.Pad[s]+PL4+BS4/2
}

func (p *Pong4) Rel(s int) float64 {
	return clamp((p.along(s)-(p.Pad[s]+PL4/2))/(PL4/2+BS4/2), -1, 1)
}

// snapTo deja la bola justo sobre la línea de ese lado.
func (p *Pong4) snapTo(s int) {
	switch s {
	case SBottom:
		p.Y = lineOf(s) - BS4
	case STop:
		p.Y = lineOf(s)
	case SLeft:
		p.X = lineOf(s)
	case SRight:
		p.X = lineOf(s) - BS4
	}
}

// bounceWall: rebote en una pared (lado sin jugador o esquina).
func (p *Pong4) bounceWall(s int) {
	p.snapTo(s)
	nx, ny := sideNormal(s)
	if nx != 0 {
		p.VX = -p.VX
	} else {
		p.VY = -p.VY
	}
	// que no quede rebotando casi paralela a una pared para siempre
	if n := math.Abs(p.VX*nx + p.VY*ny); n < .3*p.Sp {
		sp := p.Sp
		if nx != 0 {
			p.VX = -nx * .3 * sp
			p.VY = math.Copysign(math.Sqrt(sp*sp-p.VX*p.VX), p.VY)
		} else {
			p.VY = -ny * .3 * sp
			p.VX = math.Copysign(math.Sqrt(sp*sp-p.VY*p.VY), p.VX)
		}
	}
	p.emit(EvWall)
	p.anchor()
}

// Hit: la paleta de ese lado devuelve la bola (rel: dónde pegó, -1..1).
func (p *Pong4) Hit(s int, rel float64) {
	p.snapTo(s)
	p.Sp = math.Min(p.Sp*1.09, MaxSp4)
	a := rel*.95 + (rand.Float64()-.5)*.12 // un poquito de efecto: nunca queda yendo y viniendo recta
	nx, ny := sideNormal(s)
	tx, ty := math.Abs(ny), math.Abs(nx) // a lo largo del lado (derecha o abajo)
	p.VX = (-nx*math.Cos(a) + tx*math.Sin(a)) * p.Sp
	p.VY = (-ny*math.Cos(a) + ty*math.Sin(a)) * p.Sp
	p.Hits++
	p.Pending = -1
	p.emit(EvHit1)
	p.newErr()
	p.anchor()
}

func (p *Pong4) newErr() {
	for i := 0; i < 4; i++ {
		e := p.CPUErr[i]
		if e == 0 {
			e = 1.15
		}
		p.cpuErr[i] = (rand.Float64() - .5) * PL4 * e * math.Max(1, p.Sp/320) // más rápida, más le cuesta
	}
}

// loseLife: a ese lado se le pasó la bola.
func (p *Pong4) loseLife(s int) {
	p.Lives[s]--
	p.Lost = s
	p.Pending = -1
	p.emit(EvPoint)
	if p.AliveCount() <= 1 {
		p.Ph, p.Winner = PhOver, -1
		for i := 0; i < 4; i++ {
			if p.Alive(i) {
				p.Winner = i
			}
		}
		p.emit(EvWin)
	} else {
		p.Ph, p.Tm = PhServe, 1.1
	}
	p.X, p.Y, p.VX, p.VY = F4/2-BS4/2, F4/2-BS4/2, 0, 0
	p.anchor()
}

// CPUDir: hacia dónde mueve su paleta la CPU de ese lado.
func (p *Pong4) CPUDir(s int) float64 {
	target := F4 / 2
	nx, ny := sideNormal(s)
	if p.Ph == PhPlay && p.VX*nx+p.VY*ny > 0 { // viene hacia mí: adivino dónde llega
		var dist, v, pos float64
		switch s {
		case SBottom, STop:
			dist, v, pos = math.Abs(lineOf(s)-(p.Y+BS4/2)), p.VX, p.X+BS4/2
			if p.VY != 0 {
				pos += v * dist / math.Abs(p.VY)
			}
		default:
			dist, v, pos = math.Abs(lineOf(s)-(p.X+BS4/2)), p.VY, p.Y+BS4/2
			if p.VX != 0 {
				pos += v * dist / math.Abs(p.VX)
			}
		}
		for pos < 0 || pos > F4 { // rebota en las paredes de los costados
			if pos < 0 {
				pos = -pos
			}
			if pos > F4 {
				pos = 2*F4 - pos
			}
		}
		target = pos + p.cpuErr[s]
	}
	sp := p.CPUSp[s]
	if sp == 0 {
		sp = .8
	}
	return clamp((target-(p.Pad[s]+PL4/2))/12, -sp, sp)
}

// MovePad mueve la paleta de ese lado (dir -1..1).
func (p *Pong4) MovePad(s int, dir, dt float64) {
	p.Pad[s] = clamp(p.Pad[s]+dir*PSpd4*dt, Corner4-4, F4-Corner4+4-PL4)
}

// Step avanza la partida. dirs: cómo se mueve cada paleta (las remotas se ponen desde afuera).
func (p *Pong4) Step(dirs [4]float64, move [4]bool, dt float64) {
	if p.Paused || p.Ph == PhDC {
		return
	}
	for i := 0; i < 4; i++ {
		if move[i] && p.Alive(i) {
			p.MovePad(i, dirs[i], dt)
		}
	}
	if p.Predict { // invitado: solo la bola (el resto lo decide el anfitrión)
		if p.Ph == PhPlay {
			p.moveBall(dt)
		}
		return
	}
	p.T += dt
	switch p.Ph {
	case PhCount:
		prev := math.Ceil(p.Tm)
		p.Tm -= dt
		if math.Ceil(p.Tm) != prev && p.Tm > 0 {
			p.emit(EvTick)
		}
		if p.Tm <= 0 {
			p.serve()
		}
	case PhServe:
		p.Tm -= dt
		if p.Tm <= 0 {
			p.serve()
		}
	case PhPlay:
		if p.Pending >= 0 {
			if p.T-p.pendAt > p.Grace { // no llegó su aviso: se le pasó
				p.loseLife(p.Pending)
			}
			return // la bola espera en su línea
		}
		p.moveBall(dt)
	}
}

// moveBall mueve la bola en pasitos y resuelve paredes, paletas y puntos.
func (p *Pong4) moveBall(dt float64) {
	n := int(math.Ceil(p.Sp * dt / 3))
	if n < 1 {
		n = 1
	}
	for k := 0; k < n; k++ {
		p.X += p.VX * dt / float64(n)
		p.Y += p.VY * dt / float64(n)
		for s := 0; s < 4; s++ {
			if !p.reach(s) {
				continue
			}
			c := p.along(s)
			corner := c < Corner4 || c > F4-Corner4
			if !corner && p.Alive(s) && p.Pending != s {
				p.Reached[s]++
			}
			switch {
			case corner || !p.Alive(s) && !p.Predict:
				p.bounceWall(s)
			case p.Predict && p.Kind[s] == SlotEmpty:
				p.bounceWall(s)
			case p.Predict && s == p.Mine:
				if p.Covers(s) { // mi golpe: lo decido yo y lo aviso
					rel := p.Rel(s)
					p.Hit(s, rel)
					if p.OnHit != nil {
						p.OnHit(s, rel)
					}
				} else if p.out(s) {
					p.VX, p.VY = 0, 0 // se me pasó: espero lo que diga el anfitrión
				}
			case p.Predict && p.Lives[s] <= 0:
				p.bounceWall(s)
			case p.Predict: // la paleta de otro: la veo con atraso, así que supongo que la devuelve
				p.Hit(s, p.Rel(s)) // (si no, el anfitrión avisa enseguida y la bola vuelve al centro)
			case p.Covers(s):
				p.Hit(s, p.Rel(s))
			case p.Remote[s] && p.Pending < 0:
				p.Pending, p.pendAt = s, p.T // ¿la golpeó en su pantalla?
				p.snapTo(s)
				return
			case p.out(s):
				p.loseLife(s)
				return
			}
		}
	}
}

// RemoteHit: llegó el aviso de golpe de un jugador remoto.
func (p *Pong4) RemoteHit(s int, rel float64) bool {
	if p.Ph != PhPlay || !p.Alive(s) {
		return false
	}
	nx, ny := sideNormal(s)
	toward := p.VX*nx+p.VY*ny > 0
	var d float64
	switch s {
	case SBottom, STop:
		d = math.Abs(p.Y + BS4/2 - lineOf(s))
	default:
		d = math.Abs(p.X + BS4/2 - lineOf(s))
	}
	if p.Pending == s {
		waited := p.T - p.pendAt
		p.Hit(s, rel)
		// la golpeó a tiempo en su pantalla: la bola recupera lo que esperó el aviso (así los
		// demás, que ya la vieron rebotar, no la ven volver atrás)
		p.moveBall(waited)
		p.anchor()
		return true
	}
	if toward && d < 45 {
		p.Hit(s, rel)
		return true
	}
	return false
}

// AdvanceFrom: dónde está la bola `age` segundos después del ancla, en línea recta.
func AdvanceFrom(ax, ay, vx, vy, age float64) (float64, float64) {
	x, y := ax+vx*age, ay+vy*age
	return clamp(x, -BS4, F4), clamp(y, -BS4, F4)
}
