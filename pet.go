package main

import (
	"image/color"
	"math"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// La mascota: tu personaje quieto en una esquina de la pantalla, con pequeñas animaciones
// (mira, salta, silba, bosteza, se duerme...). Se arrastra con el mouse, cuelga como si lo
// hubieran levantado y, si lo sueltas en el basurero, Idle se cierra.

type particle struct {
	x, y, vx, vy float64
	s            string
	col          color.NRGBA
	born         time.Time
	life         float64
	size         float64
}

type Pet struct {
	cx, by float64 // centro de los pies, en la pantalla
	sc     float64 // tamaño de cada "pixel" del personaje

	act      string // actividad actual
	actAt    time.Time
	actLen   float64
	nextAct  time.Time
	lastPoke time.Time // última vez que lo tocaste
	asleep   bool
	flip     bool

	hoverSince time.Time
	nextHeart  time.Time
	nextZ      time.Time
	nextNote   time.Time

	pressed, drag  bool
	downAt         time.Time
	downX, downY   float64
	ang, angV, lvx float64 // balanceo al colgar
	liftAt         time.Time
	landAt         time.Time
	dyingAt        time.Time
	dieFromX       float64
	dieFromY       float64
	overTrash      bool

	parts []particle
	last  time.Time
	buf   *ebiten.Image // para dibujarlo girado mientras cuelga
}

func NewPet(cx, by float64) *Pet {
	now := time.Now()
	return &Pet{cx: cx, by: by, sc: 5, lastPoke: now, nextAct: now.Add(4 * time.Second), last: now}
}

// tick: reloj para respirar y parpadear (60 por segundo, aunque se dibuje menos seguido).
func animTick() int { return int(time.Since(clockStart).Seconds() * 60) }

// Bounds: rectángulo que ocupa (con gorro), para saber si el mouse está encima.
func (p *Pet) Bounds() (x0, y0, x1, y1 float64) {
	return p.cx - 6*p.sc, p.by - 11*p.sc, p.cx + 6*p.sc, p.by
}

func (p *Pet) Hit(x, y float64) bool {
	x0, y0, x1, y1 := p.Bounds()
	m := 6.0
	return x >= x0-m && x <= x1+m && y >= y0-m && y <= y1+m
}

func (p *Pet) Dying() bool { return !p.dyingAt.IsZero() }

func (p *Pet) Busy() bool {
	return p.pressed || p.drag || p.Dying() || len(p.parts) > 0 || p.act != "" ||
		time.Since(p.landAt) < time.Second
}

func (p *Pet) poke() {
	p.lastPoke = time.Now()
	if p.asleep {
		p.asleep = false
		p.emit("!", colAcc, 16)
	}
}

func (p *Pet) emit(s string, c color.NRGBA, size float64) {
	p.parts = append(p.parts, particle{x: p.cx + (rand.Float64()-.5)*p.sc*4, y: p.by - 12*p.sc,
		vx: (rand.Float64() - .5) * 12, vy: -22 - rand.Float64()*10, s: s, col: c, born: time.Now(), life: 1.6, size: size})
}

// Actividades al azar (todas cortas y sin moverse de su lugar).
var petActs = []struct {
	name string
	len  float64
	w    int
}{{"mirar", 2.6, 4}, {"saltito", .7, 2}, {"silbar", 3.2, 2}, {"bostezo", 1.6, 2}, {"estirar", 1.4, 2}, {"girar", .1, 2}}

func pickAct() (string, float64) {
	tot := 0
	for _, a := range petActs {
		tot += a.w
	}
	r := rand.Intn(tot)
	for _, a := range petActs {
		if r < a.w {
			return a.name, a.len
		}
		r -= a.w
	}
	return "mirar", 2
}

// Update avanza la mascota. Devuelve click (la tocaste sin arrastrar), released (terminó
// una interacción con el mouse) y trashed (la soltaste en el basurero).
func (p *Pet) Update(mx, my float64, down, anims bool, sw, sh, tx, ty float64) (click, released, trashed bool) {
	now := time.Now()
	dt := math.Min(.1, now.Sub(p.last).Seconds())
	p.last = now
	if p.Dying() {
		return
	}
	over := p.Hit(mx, my)

	// mouse: apretar, arrastrar, soltar
	if down && !p.pressed && over {
		p.pressed, p.downAt, p.downX, p.downY = true, now, mx, my
		p.poke()
	}
	if p.pressed && down {
		if !p.drag && (math.Hypot(mx-p.downX, my-p.downY) > 5 || now.Sub(p.downAt) > 350*time.Millisecond) {
			p.drag, p.liftAt, p.ang, p.angV = true, now, 0, 0
			p.act = ""
			p.emit("!", colAcc, 16)
		}
		if p.drag { // cuelga de la cabeza, un poquito detrás del mouse
			prev := p.cx
			p.cx += (mx - p.cx) * .5
			p.by += (my + 11*p.sc - p.by) * .5
			vx := (p.cx - prev) / math.Max(dt, .001)
			p.lvx += (vx - p.lvx) * .3
			target := clamp(-p.lvx*.0016, -.7, .7) // se balancea hacia atrás al moverlo
			p.angV += (target - p.ang) * .22
			p.angV *= .8
			p.ang += p.angV
			p.overTrash = math.Hypot(p.cx-tx, p.by-6*p.sc-ty) < 80
		}
	}
	if p.pressed && !down {
		p.pressed, released = false, true
		if p.drag {
			p.drag = false
			if p.overTrash {
				p.dyingAt, p.dieFromX, p.dieFromY, trashed = now, p.cx, p.by, true
				return
			}
			p.cx = clamp(p.cx, 6*p.sc+4, sw-6*p.sc-4)
			p.by = clamp(p.by, 12*p.sc, sh-4)
			p.landAt, p.ang, p.angV = now, 0, 0
		} else {
			click = true
		}
	}

	// cariño: el mouse quieto encima lo pone contento
	if over && !p.drag {
		if p.hoverSince.IsZero() {
			p.hoverSince = now
		}
		if p.asleep {
			p.poke()
		}
		if anims && now.Sub(p.hoverSince) > 1200*time.Millisecond && now.After(p.nextHeart) {
			p.emit("♥", hex(0xe0796b), 16)
			p.nextHeart = now.Add(700 * time.Millisecond)
			p.lastPoke = now
		}
	} else {
		p.hoverSince = time.Time{}
	}

	// actividades y sueño
	if anims && !p.drag && !p.pressed {
		if p.act != "" && now.Sub(p.actAt).Seconds() > p.actLen {
			if p.act == "girar" {
				p.flip = !p.flip
			}
			p.act = ""
		}
		if !p.asleep && p.act == "" && now.Sub(p.lastPoke) > 150*time.Second {
			p.asleep = true
		}
		if !p.asleep && p.act == "" && now.After(p.nextAct) {
			p.act, p.actLen = pickAct()
			p.actAt = now
			p.nextAct = now.Add(time.Duration((p.actLen + 5 + rand.Float64()*8) * float64(time.Second)))
		}
		if p.asleep && now.After(p.nextZ) {
			p.emit([]string{"z", "Z"}[rand.Intn(2)], colInk, 8+8*float64(rand.Intn(2)))
			p.nextZ = now.Add(2600 * time.Millisecond)
		}
		if p.act == "silbar" && now.After(p.nextNote) {
			p.emit("♪", hex(0xf2c94c), 16)
			p.nextNote = now.Add(600 * time.Millisecond)
		}
	} else if !anims {
		p.act, p.asleep = "", false
	}

	// partículas (z, ♪, ♥, !) que suben y se desvanecen
	keep := p.parts[:0]
	for _, q := range p.parts {
		if now.Sub(q.born).Seconds() < q.life {
			keep = append(keep, q)
		}
	}
	p.parts = keep
	return
}

// pose: cómo se ve en este momento, y cuánto se corre del lugar (saltito).
func (p *Pet) pose(mx, my float64, anims bool) (Pose, float64) {
	pose := normalPose
	pose.Still = !anims
	now := time.Now()
	dy := 0.0
	if p.drag {
		pose.Dangle = true
		if now.Sub(p.liftAt) < 700*time.Millisecond {
			pose.Eyes, pose.Mouth = 1, true // ¡me levantaron!
		} else {
			pose.Eyes = 2
		}
		return pose, 0
	}
	if k := now.Sub(p.landAt).Seconds() / .35; k < 1 { // aterriza: se aplasta y rebota
		s := math.Sin(math.Pi * k)
		pose.SY, pose.SX = 1-.28*s, 1+.18*s
		return pose, 0
	}
	if p.asleep {
		pose.Eyes = 3
		return pose, 0
	}
	k := 0.0
	if p.act != "" {
		k = clamp(now.Sub(p.actAt).Seconds()/p.actLen, 0, 1)
	}
	switch p.act {
	case "mirar":
		pose.EyeDX = -1
		if k > .5 {
			pose.EyeDX = 1
		}
	case "saltito":
		dy = -math.Sin(math.Pi*k) * 2.5 * p.sc
		pose.SY = 1 + .08*math.Sin(math.Pi*k)
	case "silbar":
		pose.Eyes = 2
	case "bostezo":
		pose.Eyes, pose.Mouth = 3, true
		pose.SY = 1 + .06*math.Sin(math.Pi*k)
	case "estirar":
		pose.SY, pose.SX = 1+.14*math.Sin(math.Pi*k), 1-.08*math.Sin(math.Pi*k)
	}
	if d := mx - p.cx; anims && math.Hypot(d, my-(p.by-6*p.sc)) < 170 { // mira hacia el mouse
		pose.EyeDX = int(clamp(math.Round(d/40), -1, 1))
		if p.Hit(mx, my) {
			pose.Eyes = 2
		}
	}
	return pose, dy
}

func (p *Pet) Draw(s *ebiten.Image, av Avatar, mx, my float64, anims bool, tx, ty float64) {
	t := animTick()
	sc := p.sc
	if p.Dying() { // cae en el basurero
		k := clamp(time.Since(p.dyingAt).Seconds()/.6, 0, 1)
		x := p.dieFromX + (tx-p.dieFromX)*k
		y := p.dieFromY + (ty+20-p.dieFromY)*k
		sc *= 1 - .85*k
		pose := normalPose
		pose.Eyes, pose.Dangle = 1, true
		drawAvatarPose(s, x-6*sc, y-8*sc, sc, av, t, p.flip, nil, pose)
		return
	}
	pose, dy := p.pose(mx, my, anims)
	x, y := p.cx-6*sc, p.by-8*sc+dy
	if p.drag {
		if p.buf == nil {
			p.buf = ebiten.NewImage(int(16*sc), int(16*sc))
		}
		p.buf.Clear()
		drawAvatarPose(p.buf, 2*sc+2, 4*sc+2, sc, av, t, p.flip, shadowCol, pose)
		drawAvatarPose(p.buf, 2*sc, 4*sc, sc, av, t, p.flip, nil, pose)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(-8*sc, -1*sc) // gira desde la cabeza, donde lo agarra el mouse
		op.GeoM.Rotate(p.ang)
		op.GeoM.Translate(p.cx+drawOX, p.by-11*sc+drawOY)
		s.DrawImage(p.buf, op)
	} else {
		rect(s, p.cx-5*sc, p.by-1, 10*sc, 3, color.NRGBA{0, 0, 0, 60}) // sombrita en el piso
		drawAvatarPose(s, x+2, y+2, sc, av, t, p.flip, shadowCol, pose)
		drawAvatarPose(s, x, y, sc, av, t, p.flip, nil, pose)
	}
	now := time.Now()
	for _, q := range p.parts {
		age := now.Sub(q.born).Seconds()
		a := clamp(1-age/q.life, 0, 1)
		stxt(s, q.s, q.x+q.vx*age, q.y+q.vy*age, q.size, fade(q.col, a), 0)
	}
}

// drawTrash dibuja el basurero (aparece mientras arrastras la mascota).
func drawTrash(s *ebiten.Image, cx, cy float64, hot bool, closing float64) {
	c := hex(0x9a9690)
	if hot {
		c = colErr
	}
	k := 5.0
	x, y := cx-6*k, cy-7*k
	rect(s, x+3, y+3, 12*k, 14*k, shadowCol)
	rect(s, x+k, y+3*k, 10*k, 11*k, c) // cuerpo
	for i := 0; i < 3; i++ {
		rect(s, x+(3+float64(i)*2.5)*k, y+5*k, k*.8, 7*k, shade(c, .7))
	}
	lift := 0.0
	if hot && closing == 0 {
		lift = 2 * k // la tapa se abre
	}
	rect(s, x, y+1.5*k-lift, 12*k, k*1.2, c) // tapa
	rect(s, x+4.5*k, y+.5*k-lift, 3*k, k, c)
}
