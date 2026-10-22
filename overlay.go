package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"runtime"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Modo sobre la pantalla: una ventana transparente del tamaño del monitor, siempre encima y que
// deja pasar los clics a lo que haya debajo. Solo se ven los personajes, la bola y el marcador.
//
// Vista previa: todavía no captura el teclado, así que muestra una partida de demostración
// (tu personaje contra la CPU) para ver cómo se ve sobre tu pantalla.

type Overlay struct {
	cfg     Config
	p       *Pong
	tick    int
	sw, sh  int
	leftErr float64 // error de puntería del personaje izquierdo (la demo juega sola)
	lastHit uint8
	overAt  int
	trail   [][2]float64
	pulse1  float64
	pulse2  float64
	lastEv  [EvCount]uint8
}

func NewOverlay() *Overlay {
	return &Overlay{cfg: loadCfg(), p: NewPong()}
}

func (o *Overlay) Layout(ow, oh int) (int, int) {
	o.sw, o.sh = ow, oh
	return ow, oh
}

// leftAI: el personaje izquierdo sigue la bola con algo de error (igual que la CPU de la derecha).
func (o *Overlay) leftAI() Input {
	p := o.p
	if p.Ev[EvHit2] != o.lastHit { // cada vez que el otro le pega, cambia su puntería
		o.lastHit = p.Ev[EvHit2]
		o.leftErr = (rand.Float64() - .5) * PH * 1.2
	}
	c := p.A + PH/2
	t := float64(H / 2)
	if p.Ph == PhPlay && p.VX < 0 {
		t = p.Y + BS/2 + o.leftErr
	}
	return Input{Dir: clamp((t-c)/12, -.8, .8)}
}

func (o *Overlay) Update() error {
	o.tick++
	cmd := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if pressed(ebiten.KeyEscape) || cmd && pressed(ebiten.KeyQ) {
		return ebiten.Termination
	}
	p := o.p
	in2 := p.CPUInput()
	p.Step(o.leftAI(), &in2, Dt)
	if p.Ph == PhOver {
		if o.overAt == 0 {
			o.overAt = o.tick
		} else if o.tick-o.overAt > 240 { // 4 s mostrando al ganador y otra partida
			p.Reset()
			o.overAt = 0
		}
	}
	if p.Ev[EvHit1] != o.lastEv[EvHit1] {
		o.pulse1 = 1
	}
	if p.Ev[EvHit2] != o.lastEv[EvHit2] {
		o.pulse2 = 1
	}
	o.lastEv = p.Ev
	o.pulse1 = math.Max(0, o.pulse1-Dt*4)
	o.pulse2 = math.Max(0, o.pulse2-Dt*4)
	if p.Ph == PhPlay {
		o.trail = append(o.trail, [2]float64{p.X, p.Y})
		if len(o.trail) > 6 {
			o.trail = o.trail[1:]
		}
	} else {
		o.trail = o.trail[:0]
	}
	return nil
}

var shadowCol = color.NRGBA{0, 0, 0, 110}

// sombra + figura, para que se lea sobre cualquier fondo (claro u oscuro).
func srect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	rect(dst, x+2, y+2, w, h, shadowCol)
	rect(dst, x, y, w, h, c)
}

func stxt(dst *ebiten.Image, s string, x, y, size float64, c color.Color, align int) {
	txt(dst, s, x+2, y+2, size, color.NRGBA{0, 0, 0, 150}, align)
	txt(dst, s, x, y, size, c, align)
}

// ptxt: texto sobre una pastilla oscura semitransparente (se lee sobre ventanas claras).
func ptxt(dst *ebiten.Image, s string, x, y, size float64, c color.Color, align int) {
	w := text.Advance(s, &text.GoTextFace{Source: fontSrc, Size: size})
	left := x - w/2
	switch align {
	case -1:
		left = x
	case 1:
		left = x - w
	}
	pad := size / 2
	rect(dst, left-pad, y-pad*.8, w+2*pad, size+pad*1.6, color.NRGBA{20, 20, 19, 170})
	txt(dst, s, x, y, size, c, align)
}

var ballCol = hex(0xf2c94c) // amarillo con borde oscuro: se ve sobre fondos claros y oscuros

// ball dibuja un cuadrado con borde oscuro.
func ball(dst *ebiten.Image, x, y, w float64, c color.Color) {
	b := math.Max(2, w/6)
	rect(dst, x+2, y+2, w, w, shadowCol)
	rect(dst, x-b, y-b, w+2*b, w+2*b, color.NRGBA{25, 25, 25, 220})
	rect(dst, x, y, w, w, c)
}

func (o *Overlay) Draw(s *ebiten.Image) {
	p := o.p
	sw, sh := float64(o.sw), float64(o.sh)
	k := math.Min(sw/W, sh/H) // escala: la cancha usa toda la pantalla
	ox, oy := (sw-W*k)/2, (sh-H*k)/2
	X := func(x float64) float64 { return math.Round(ox + x*k) }
	Y := func(y float64) float64 { return math.Round(oy + y*k) }

	av1, av2 := o.cfg.Av, avCPU
	c1, c2 := av1.Col(), av2.Col()

	// paletas, cada una sostenida por su personaje
	srect(s, X(PX), Y(p.A), PW*k, PH*k, c1)
	srect(s, X(W-PX-PW), Y(p.B), PW*k, PH*k, c2)
	ax, ay := X(3+o.pulse1), Y(p.A+PH/2-4)
	bx, by := X(W-15-o.pulse2), Y(p.B+PH/2-4)
	drawAvatarTint(s, ax+2, ay+2, k, av1, o.tick, false, shadowCol)
	drawAvatar(s, ax, ay, k, av1, o.tick, false)
	drawAvatarTint(s, bx+2, by+2, k, av2, o.tick+40, true, shadowCol)
	drawAvatar(s, bx, by, k, av2, o.tick+40, true)

	// bola y su estela
	if p.Ph == PhPlay {
		for i, t := range o.trail {
			al := float64(i+1) / float64(len(o.trail)) * .35
			rect(s, X(t[0]+1), Y(t[1]+1), (BS-2)*k, (BS-2)*k, fade(shade(ballCol, .8), al))
		}
		ball(s, X(p.X), Y(p.Y), BS*k, ballCol)
	}

	// marcador chico arriba al centro
	score := fmt.Sprintf("%s %d · %d %s", av1.DisplayName(), p.S1, p.S2, av2.DisplayName())
	ptxt(s, score, sw/2, 16, 16, colInk, 0)

	switch p.Ph {
	case PhCount:
		ptxt(s, strconv.Itoa(int(math.Max(1, math.Ceil(p.Tm)))), sw/2, sh/2-16, 32, colInk, 0)
	case PhOver:
		w := av1
		if p.Winner == 2 {
			w = av2
		}
		ptxt(s, "GANA "+w.DisplayName(), sw/2, sh/2-8, 16, w.Col(), 0)
	}
	ptxt(s, "IDLE · VISTA PREVIA · ESC CIERRA", sw-20, sh-30, 8, fade(colInk, .8), 1)
	shot(s, o.tick)
}

// runOverlay abre la ventana transparente sobre toda la pantalla.
func runOverlay() error {
	mw, mh := 1280, 800
	if m := ebiten.Monitor(); m != nil {
		mw, mh = m.Size()
	}
	if runtime.GOOS == "windows" {
		mh-- // si ocupa exacto el monitor, Windows la trata como pantalla completa y esconde la barra de tareas
	}
	ebiten.SetWindowTitle("Idle")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(true)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowSize(mw, mh)
	ebiten.SetWindowPosition(0, 0)
	return ebiten.RunGameWithOptions(NewOverlay(), &ebiten.RunGameOptions{ScreenTransparent: true, InitUnfocused: true})
}
