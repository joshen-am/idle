package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pong de 4: sala de espera (panel) y cancha cuadrada sobre la pantalla.

func (o *Overlay) drawLobby4(s *ebiten.Image) {
	m := o.m4
	w, h := 440.0, 300.0
	x, y := o.panel(s, w, h)
	txt(s, "PONG X4 · SALA "+o.cfg.Room, x+16, y+12, 8, colDim, -1)
	txt(s, m.Status(), x+16, y+30, 8, colAcc, -1)
	for i, side := range fillOrder {
		ry := y + 52 + float64(i)*40
		sl := m.Slots[side]
		rect(s, x+10, ry, w-20, 36, colCard)
		txt(s, sideNames[side], x+20, ry+14, 8, colDim, -1)
		switch sl.Kind {
		case SlotEmpty:
			txt(s, "VACIO (PARED)", x+130, ry+14, 8, colLine, -1)
		default:
			drawAvatar(s, x+130, ry+10, 2, sl.Av, o.tick+i*20, false)
			txt(s, sl.Av.DisplayName(), x+164, ry+10, 16, sl.Av.Col(), -1)
			tag := "CPU"
			if sl.Kind == SlotHuman {
				tag = "JUGADOR"
				if side == m.MySide {
					tag = "TU"
				}
			}
			txt(s, tag, x+w-24, ry+14, 8, colDim, 1)
			if sl.Kind == SlotCPU && m.Host() {
				b := o.button(fmt.Sprintf("x4:rm:%d", side), x+w-90, ry+6, 24, 24)
				txt(s, "×", b.x+b.w/2, b.y+4, 16, colErr, 0)
			}
		}
	}
	by := y + h - 44
	if m.Host() {
		b := o.button("x4:cpu", x+10, by, 130, 30)
		c := colInk
		if m.Players() >= 4 {
			c = colLine
		}
		rect(s, b.x, b.y, b.w, b.h, colCard)
		frame(s, b.x, b.y, b.w, b.h, colLine)
		txt(s, "+ CPU", b.x+b.w/2, b.y+8, 16, c, 0)
		b = o.button("x4:go", x+150, by, 150, 30)
		if m.CanStart() {
			rect(s, b.x, b.y, b.w, b.h, colAcc)
			txt(s, "EMPEZAR", b.x+b.w/2, b.y+8, 16, colBg, 0)
		} else {
			frame(s, b.x, b.y, b.w, b.h, colLine)
			txt(s, "EMPEZAR", b.x+b.w/2, b.y+8, 16, colLine, 0)
		}
	} else {
		txt(s, "EL ANFITRION ARMA LA PARTIDA", x+16, by+10, 8, colDim, -1)
	}
	b := o.button("salir", x+w-110, by, 100, 30)
	rect(s, b.x, b.y, b.w, b.h, colCard)
	frame(s, b.x, b.y, b.w, b.h, colLine)
	txt(s, "SALIR", b.x+b.w/2, b.y+8, 16, colInk, 0)
}

// keysFor: con qué flechas se mueve la paleta de ese lado.
func keysFor(side int) string {
	if side == SLeft || side == SRight {
		return "↑ ↓"
	}
	return "← →"
}

func (o *Overlay) drawMatch4(s *ebiten.Image) {
	m := o.m4
	p := m.p
	sw, sh := float64(o.sw), float64(o.sh)
	k := math.Floor(math.Min(sw, sh) * .8 / F4 * 4) / 4
	ox, oy := math.Round((sw-F4*k)/2), math.Round((sh-F4*k)/2)
	X := func(v float64) float64 { return math.Round(ox + v*k) }
	Y := func(v float64) float64 { return math.Round(oy + v*k) }

	rect(s, ox, oy, F4*k, F4*k, color.NRGBA{20, 20, 19, 70}) // cancha apenas marcada
	wallCol := color.NRGBA{120, 116, 108, 200}
	// esquinas (siempre pared)
	for _, c := range [][2]float64{{0, 0}, {F4 - Corner4, 0}, {0, F4 - Corner4}, {F4 - Corner4, F4 - Corner4}} {
		rect(s, X(c[0]), Y(c[1]), Corner4*k, Corner4*k, color.NRGBA{60, 58, 54, 80})
		frame(s, X(c[0]), Y(c[1]), Corner4*k, Corner4*k, color.NRGBA{120, 116, 108, 160})
	}
	for side := 0; side < 4; side++ {
		sl := m.Slots[side]
		alive := p.Alive(side) || !m.Started && sl.Kind != SlotEmpty
		col := sl.Av.Col()
		if f := m.flash[side]; f > 0 && int(f*10)%2 == 0 {
			col = colErr
		}
		// paleta o pared
		var px, py, pw, ph float64
		switch side {
		case SBottom, STop:
			py = lineOf(side)
			if side == SBottom {
				py -= 0
			} else {
				py -= PW4
			}
			px, pw, ph = p.Pad[side], PL4, PW4
			if !alive {
				px, pw = Corner4, F4-2*Corner4
			}
		default:
			px = lineOf(side)
			if side == SLeft {
				px -= PW4
			}
			py, pw, ph = p.Pad[side], PW4, PL4
			if !alive {
				py, ph = Corner4, F4-2*Corner4
			}
		}
		if !alive {
			srect(s, X(px), Y(py), pw*k, ph*k, wallCol)
			continue
		}
		if f := m.flash[side]; f < 0 { // acaba de golpear
			col = color.NRGBA{col.R + (255-col.R)/2, col.G + (255-col.G)/2, col.B + (255-col.B)/2, 255}
		}
		srect(s, X(px), Y(py), pw*k, ph*k, col)

		// personaje, nombre y vidas, afuera de la cancha detrás de su paleta
		sc := k * .9
		var ax, ay float64
		c := p.Pad[side] + PL4/2
		switch side {
		case SBottom:
			ax, ay = X(c)-6*sc, Y(F4)+4*sc
		case STop:
			ax, ay = X(c)-6*sc, Y(0)-12*sc
		case SLeft:
			ax, ay = X(0)-16*sc, Y(c)-4*sc
		case SRight:
			ax, ay = X(F4)+4*sc, Y(c)-4*sc
		}
		drawAvatarTint(s, ax+2, ay+2, sc, sl.Av, o.tick+side*30, side == SRight, shadowCol)
		drawAvatar(s, ax, ay, sc, sl.Av, o.tick+side*30, side == SRight)
		hearts := strings.Repeat("♥", max(p.Lives[side], 0)) + strings.Repeat("·", max(Lives4-p.Lives[side], 0))
		label := sl.Av.DisplayName()
		if side == m.MySide {
			label = "TU " + keysFor(side)
		}
		lx, ly, al := ax+6*sc, ay+10*sc, 0
		switch side {
		case STop:
			ly = ay - 26
		case SLeft:
			lx, ly, al = ax+12*sc, ay+10*sc, 1
		case SRight:
			lx, al = ax, -1
		}
		lc := colInk
		if side == m.MySide {
			lc = colAcc
		}
		ptxt(s, label, lx, ly, 8, lc, al)
		ptxt(s, hearts, lx, ly+14, 8, colErr, al)
	}

	if p.Ph == PhPlay {
		for i, t := range m.trail {
			al := float64(i+1) / float64(len(m.trail)) * .35
			rect(s, X(t[0]+1), Y(t[1]+1), (BS4-2)*k, (BS4-2)*k, fade(shade(ballCol, .8), al))
		}
		ball(s, X(p.X), Y(p.Y), BS4*k, ballCol)
		// velocímetro: va subiendo
		ptxt(s, fmt.Sprintf("VELOCIDAD %d", int(p.Sp/10)), ox+F4*k, oy-24, 8, colDim, 1)
	}
	cx, cy := sw/2, sh/2
	switch {
	case p.Ph == PhCount:
		ptxt(s, fmt.Sprint(int(math.Max(1, math.Ceil(p.Tm)))), cx, cy-16, 32, colInk, 0)
		ptxt(s, "↑ Y ↓ JUNTAS ×2 = TRABAJO", cx, cy+40, 8, colDim, 0)
	case p.Ph == PhOver:
		if p.Winner >= 0 {
			w := m.Slots[p.Winner].Av
			ptxt(s, "GANA "+w.DisplayName(), cx, cy-24, 16, w.Col(), 0)
		}
		if o.coinMsg != "" {
			ptxt(s, o.coinMsg, cx, cy-60, 16, colAcc, 0)
		}
		if m.Host() {
			o.bigButton(s, "revancha", "REVANCHA", cx-170, cy+20, 160, colAcc)
		} else {
			ptxt(s, "EL ANFITRION DECIDE LA REVANCHA", cx, cy+70, 8, colDim, 0)
		}
		o.bigButton(s, "salir", "SALIR", cx+10, cy+20, 160, colCard)
	case p.Ph == PhDC:
		ptxt(s, "SE CORTO LA CONEXION", cx, cy-24, 16, colErr, 0)
		o.bigButton(s, "salir", "VOLVER", cx-80, cy+20, 160, colCard)
	case p.Paused:
		ptxt(s, "UN MOMENTO"+dots(o.tick), cx, oy-24, 8, colDim, 0)
	}
	if p.Ph != PhOver && p.Ph != PhDC {
		o.bigButton(s, "salir", "SALIR", sw-110, 10, 96, colCard)
	}
}
