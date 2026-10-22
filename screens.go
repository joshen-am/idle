package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------- colores (sobrios, estilo terminal) y letra ----------

func hex(h uint32) color.NRGBA { return color.NRGBA{uint8(h >> 16), uint8(h >> 8), uint8(h), 255} }
func fade(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(clamp(a, 0, 1) * 255)
	return c
}

var (
	colBg    = hex(0x141413)
	colPanel = hex(0x1b1a18)
	colCard  = hex(0x23211e)
	colLine  = hex(0x3a3733)
	colField = hex(0x2a2825)
	colInk   = hex(0xe6e0d4)
	colDim   = hex(0x8c867b)
	colAcc   = hex(0xd97757)
	colErr   = hex(0xe0796b)
	colOk    = hex(0x8fbf7f)
	colBall  = hex(0xe6e0d4)
)

var fontSrc *text.GoTextFaceSource

func init() {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.PressStart2P_ttf))
	if err != nil {
		log.Fatal(err)
	}
	fontSrc = s
}

func rect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), c, false)
}

func frame(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	rect(dst, x, y, w, 1, c)
	rect(dst, x, y+h-1, w, 1, c)
	rect(dst, x, y, 1, h, c)
	rect(dst, x+w-1, y, 1, h, c)
}

// txt dibuja texto; align: -1 izquierda, 0 centro, 1 derecha.
func txt(dst *ebiten.Image, s string, x, y, size float64, c color.Color, align int) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x), math.Round(y))
	op.ColorScale.ScaleWithColor(c)
	op.PrimaryAlign = [...]text.Align{text.AlignStart, text.AlignCenter, text.AlignEnd}[align+1]
	text.Draw(dst, s, &text.GoTextFace{Source: fontSrc, Size: size}, op)
}

// Dibujitos pixelados: lista de rectángulos {x, y, w, h} en una grilla.
type px [4]float64

func pix(dst *ebiten.Image, ox, oy, scale float64, rs []px, c color.Color) {
	for _, r := range rs {
		rect(dst, ox+r[0]*scale, oy+r[1]*scale, r[2]*scale, r[3]*scale, c)
	}
}

var (
	icBack  = []px{{4, 1, 1, 1}, {3, 2, 1, 1}, {2, 3, 1, 2}, {3, 5, 1, 1}, {4, 6, 1, 1}}
	icNext  = []px{{3, 1, 1, 1}, {4, 2, 1, 1}, {5, 3, 1, 2}, {4, 5, 1, 1}, {3, 6, 1, 1}}
	icSpk   = []px{{1, 3, 1, 2}, {2, 2, 1, 4}, {3, 1, 1, 6}}
	icWaves = []px{{5, 2, 1, 1}, {6, 3, 1, 2}, {5, 5, 1, 1}}
	icPos   = []px{{0, 0, 8, 1}, {0, 7, 8, 1}, {0, 1, 1, 6}, {7, 1, 1, 6}, {4, 4, 3, 3}}
	icClose = []px{{1, 1, 1, 1}, {2, 2, 1, 1}, {3, 3, 2, 2}, {5, 2, 1, 1}, {6, 1, 1, 1}, {2, 5, 1, 1}, {1, 6, 1, 1}, {5, 5, 1, 1}, {6, 6, 1, 1}}
)

func dots(t int) string { return strings.Repeat(".", (t/18)%4) }

// ---------- Draw ----------

func (a *App) Layout(int, int) (int, int) { return W, H }

func (a *App) Draw(s *ebiten.Image) {
	switch {
	case a.panic:
		s.Fill(colPanel)
		a.drawPanic(s)
		a.drawTopBar(s, "ACTIVIDAD")
	case a.scene == SGame:
		a.drawGame(s)
	default:
		s.Fill(colPanel)
		switch a.scene {
		case SMenu:
			a.drawMenu(s)
		case SPong:
			a.drawModes(s)
		case SOnline:
			a.drawOnline(s)
		case SProfile:
			a.drawProfile(s)
		}
		a.drawTopBar(s, map[Scene]string{SMenu: "IDLE", SPong: "PONG", SOnline: "ONLINE", SProfile: "PERSONAJE"}[a.scene])
		a.drawUpdate(s)
	}
	frame(s, 0, 0, W, H, colLine) // borde para que se vea el contorno de la ventana
	shot(s, a.tick)
}

func (a *App) drawUpdate(s *ebiten.Image) {
	st, v := a.upd.Status()
	msg := ""
	switch st {
	case "descargando":
		msg = "BAJANDO V" + v + dots(a.tick)
	case "listo":
		msg = "ACTUALIZANDO A V" + v
	}
	if msg == "" {
		return
	}
	rect(s, 0, H-14, W, 14, colBg)
	rect(s, 0, H-15, W, 1, colLine)
	txt(s, msg, W/2, H-11, 8, colOk, 0)
}

func (a *App) hov(b btn) bool { return b.has(a.cx, a.cy) && !a.dragging }

func (a *App) drawTopBar(s *ebiten.Image, title string) {
	rect(s, 0, 0, W, 20, colBg)
	rect(s, 0, 20, W, 1, colLine)
	cx := 100.0
	if a.scene != SMenu && !a.panic {
		cx = 106
	}
	txt(s, title, cx, 6, 8, colDim, 0)
	for _, b := range a.topButtons() {
		a.drawIconBtn(s, b, 1)
	}
}

func (a *App) drawIconBtn(s *ebiten.Image, b btn, alpha float64) {
	h := a.hov(b)
	c := colDim
	if h {
		c, alpha = colAcc, 1
	}
	rect(s, b.x, b.y, b.w, b.h, fade(colBg, alpha*.8))
	fr := colLine
	if h {
		fr = colAcc
	}
	frame(s, b.x, b.y, b.w, b.h, fade(fr, alpha))
	ox, oy := b.x+2, b.y+2
	switch {
	case b.id == "back" || strings.HasSuffix(b.id, ":-1"):
		pix(s, ox, oy, 1, icBack, fade(c, alpha))
	case strings.HasSuffix(b.id, ":1"):
		pix(s, ox, oy, 1, icNext, fade(c, alpha))
	case b.id == "mute":
		pix(s, ox, oy, 1, icSpk, fade(c, alpha))
		if !a.snd.Mute {
			pix(s, ox, oy, 1, icWaves, fade(c, alpha))
		}
	case b.id == "pos":
		pix(s, ox, oy, 1, icPos, fade(c, alpha))
	case b.id == "close":
		pix(s, ox, oy, 1, icClose, fade(c, alpha))
	}
}

func card(s *ebiten.Image, b btn, active bool) {
	rect(s, b.x, b.y, b.w, b.h, colCard)
	c := colLine
	if active {
		c = colAcc
	}
	frame(s, b.x, b.y, b.w, b.h, c)
}

// ---------- menú: tu personaje + lista de juegos ----------

func (a *App) drawMenu(s *ebiten.Image) {
	// tu personaje, a la izquierda
	rect(s, 8, 30, 70, 96, colBg)
	frame(s, 8, 30, 70, 96, colLine)
	drawAvatar(s, 19, 60, 4, a.cfg.Av, a.tick, false)
	txt(s, a.cfg.Av.DisplayName(), 43, 106, 8, a.cfg.Av.Col(), 0)

	txt(s, "> JUEGOS", 86, 30, 8, colDim, -1)
	for i, r := range menuRows {
		b := menuBtn(i)
		active := r.on && (a.hov(b) || (menuSel[a.sel%len(menuSel)] == i && !a.hoverAny()))
		if active {
			rect(s, b.x, b.y, b.w, b.h, colCard)
			txt(s, ">", b.x+4, b.y+7, 8, colAcc, -1)
		}
		c := colInk
		if !r.on {
			c = colLine
			txt(s, "PRONTO", b.x+b.w-4, b.y+7, 8, colLine, 1)
		}
		txt(s, r.label, b.x+16, b.y+7, 8, c, -1)
	}
	txt(s, "TAB OCULTA", 10, 224, 8, colLine, -1)
	txt(s, "V"+version, W-8, 224, 8, colLine, 1)
}

// ---------- editor del personaje ----------

var profLabels = []string{"CUERPO", "COLOR", "OJOS", "GORRO", "NOMBRE", "SALA"}

func (a *App) profValue(r int) string {
	av := a.cfg.Av.fix()
	switch r {
	case 0:
		return bodies[av.Body].name
	case 1:
		return avColors[av.Color].name
	case 2:
		return eyeNames[av.Eyes]
	case 3:
		return hats[av.Hat].name
	}
	return ""
}

func (a *App) drawProfile(s *ebiten.Image) {
	rect(s, 0, 21, W, 76, colBg)
	rect(s, 70, 88, 100, 1, colLine)
	drawAvatar(s, 90, 48, 5, a.cfg.Av, a.tick, false)
	for r, l := range profLabels {
		y := profY(r)
		if a.sel == r {
			rect(s, 4, y-4, W-8, 16, colCard)
			txt(s, ">", 8, y+1, 8, colAcc, -1)
		}
		txt(s, l, 18, y+1, 8, colDim, -1)
		if r >= 4 {
			cur := ""
			if a.sel == r && (a.tick/30)%2 == 0 {
				cur = "_"
			}
			val, col := a.cfg.Av.Name, a.cfg.Av.Col()
			if r == 5 {
				val, col = a.cfg.Room, colInk
			}
			txt(s, val+cur, 112, y+1, 8, col, -1)
			continue
		}
		for _, d := range []int{-1, 1} {
			a.drawIconBtn(s, profBtn(r, d), 1)
		}
		if r == 1 {
			rect(s, 122, y+1, 8, 8, a.cfg.Av.Col())
			txt(s, a.profValue(r), 172, y+1, 8, colInk, 0)
		} else {
			txt(s, a.profValue(r), 166, y+1, 8, colInk, 0)
		}
	}
	txt(s, "LA MISMA SALA EN LOS DOS PC", W/2, 222, 8, colLine, 0)
}

// ---------- pantalla de pánico: parece un monitor del sistema ----------

var fakeProcs = []struct {
	name string
	cpu  float64
}{{"KERNEL_TASK", 2.4}, {"SYNCD", .8}, {"INDEXER", .5}, {"NETWORKD", .3}}

func (a *App) drawPanic(s *ebiten.Image) {
	cur := a.cpuHist[len(a.cpuHist)-1]
	txt(s, "CPU", 12, 30, 8, colDim, -1)
	txt(s, fmt.Sprintf("%.0f%%", cur), 228, 30, 8, colInk, 1)
	rect(s, 12, 42, 216, 50, colBg)
	frame(s, 12, 42, 216, 50, colLine)
	for i, v := range a.cpuHist {
		h := math.Round(v / 100 * 46)
		rect(s, 14+float64(i)*6, 90-h, 5, h, colDim)
	}
	txt(s, "MEMORIA", 12, 102, 8, colDim, -1)
	txt(s, "6.1 GB", 228, 102, 8, colInk, 1)
	rect(s, 12, 114, 216, 6, colBg)
	rect(s, 12, 114, 132, 6, colLine)
	txt(s, "RED", 12, 128, 8, colDim, -1)
	txt(s, fmt.Sprintf("RX %d KB/S", 12+int(cur)/3), 228, 128, 8, colInk, 1)
	rect(s, 12, 142, 216, 1, colLine)
	txt(s, "PROCESO", 12, 148, 8, colDim, -1)
	txt(s, "CPU", 228, 148, 8, colDim, 1)
	for i, p := range fakeProcs {
		y := 162 + float64(i)*13
		txt(s, p.name, 12, y, 8, colInk, -1)
		txt(s, fmt.Sprintf("%.1f", p.cpu+cur/50*float64(4-i)/4), 228, y, 8, colInk, 1)
	}
	txt(s, time.Now().Format("15:04:05"), W/2, 222, 8, colLine, 0)
}

// ---------- pong: elegir modo ----------

func (a *App) drawModes(s *ebiten.Image) {
	txt(s, "PRIMERO A 7 GANA", W/2, 26, 8, colDim, 0)
	for i, m := range modeItems {
		b := btn{10, 38 + float64(i)*44, 220, 38, m.id}
		active := a.hov(b) || (a.sel == i && !a.hoverAny())
		card(s, b, active)
		switch m.id {
		case "local":
			rect(s, b.x+10, b.y+11, 4, 8, a.cfg.Av.Col())
			rect(s, b.x+14, b.y+11, 4, 8, avP2.Col())
		case "cpu":
			rect(s, b.x+10, b.y+11, 8, 8, avCPU.Col())
		default:
			rect(s, b.x+10, b.y+11, 8, 8, colAcc)
		}
		txt(s, m.title, b.x+28, b.y+9, 8, colInk, -1)
		txt(s, m.sub, b.x+28, b.y+23, 8, colDim, -1)
	}
	txt(s, "ESC VOLVER  +/- ZOOM", W/2, 224, 8, colLine, 0)
}

// ---------- online: se busca solo al compañero ----------

func (a *App) drawOnline(s *ebiten.Image) {
	st := NetConnecting
	if a.nw != nil {
		st = a.nw.Status()
	}
	drawAvatar(s, 96, 46, 4, a.cfg.Av, a.tick, false)
	txt(s, "SALA "+a.cfg.Room, W/2, 96, 8, colDim, 0)
	switch st {
	case NetWaiting:
		txt(s, "ESPERANDO A TU", W/2, 124, 8, colInk, 0)
		txt(s, "COMPAÑERO"+dots(a.tick), 76, 138, 8, colInk, -1)
		txt(s, "EN SU PC: PONG > ONLINE", W/2, 168, 8, colDim, 0)
	case NetJoining:
		txt(s, "ENTRANDO"+dots(a.tick), 84, 130, 8, colInk, -1)
	case NetNoInternet:
		txt(s, "SIN INTERNET", W/2, 124, 8, colErr, 0)
		txt(s, "REINTENTANDO"+dots(a.tick), 72, 140, 8, colDim, -1)
	default:
		txt(s, "CONECTANDO"+dots(a.tick), 80, 130, 8, colInk, -1)
	}
	txt(s, "LA SALA SE CAMBIA EN", W/2, 206, 8, colLine, 0)
	txt(s, "PERSONAJE", W/2, 218, 8, colLine, 0)
}

// ---------- el juego ----------

func (a *App) sideAvatar(side int) Avatar {
	peer := avAny
	if a.nw != nil {
		if av, ok := a.nw.PeerAvatar(); ok {
			peer = av
		}
	}
	if side == 1 {
		if a.mode == ModeGuest {
			return peer
		}
		return a.cfg.Av
	}
	switch a.mode {
	case ModeLocal:
		return avP2
	case ModeCPU:
		return avCPU
	case ModeHost:
		return peer
	}
	return a.cfg.Av
}

func (a *App) drawGame(s *ebiten.Image) {
	p := a.pong
	av1, av2 := a.sideAvatar(1), a.sideAvatar(2)
	c1, c2 := av1.Col(), av2.Col()
	s.Fill(colBg)
	ox, oy := 0.0, 0.0
	if a.shake > 0 {
		ox, oy = math.Round((rand.Float64()-.5)*4), math.Round((rand.Float64()-.5)*4)
	}
	for y := 3.0; y < H; y += 10 {
		rect(s, W/2-1+ox, y+oy, 2, 5, colField)
	}
	txt(s, strconv.Itoa(p.S1), W/2-22+ox, 10+oy, 16, c1, 1)
	txt(s, strconv.Itoa(p.S2), W/2+24+ox, 10+oy, 16, c2, -1)
	txt(s, av1.DisplayName(), W/2-22, 30, 8, fade(c1, .6), 1)
	txt(s, av2.DisplayName(), W/2+24, 30, 8, fade(c2, .6), -1)

	// paletas, cada una sostenida por su personaje
	ya, yb := math.Round(p.A)+oy, math.Round(p.B)+oy
	rect(s, PX-1+ox, ya-1, PW+2, PH+2, fade(c1, .15+a.pulse1*.3))
	rect(s, PX+ox, ya, PW, PH, c1)
	rect(s, W-PX-PW-1+ox, yb-1, PW+2, PH+2, fade(c2, .15+a.pulse2*.3))
	rect(s, W-PX-PW+ox, yb, PW, PH, c2)
	drawAvatar(s, 3+ox+math.Round(a.pulse1), ya+PH/2-4, 1, av1, a.tick, false)
	drawAvatar(s, W-15+ox-math.Round(a.pulse2), yb+PH/2-4, 1, av2, a.tick+40, true)

	bx, by := a.ballPos()
	if p.Ph == PhPlay {
		for i, t := range a.trail {
			al := float64(i+1) / float64(len(a.trail)) * .3
			rect(s, math.Round(t[0]+1)+ox, math.Round(t[1]+1)+oy, BS-2, BS-2, fade(colBall, al))
		}
		if bx < W {
			rect(s, math.Round(bx)+ox, math.Round(by)+oy, BS, BS, colBall)
		}
	} else if p.Ph == PhServe && (a.tick/7)%2 == 0 {
		rect(s, W/2-2, H/2-2, BS, BS, colBall)
	}
	if a.flash > 0 {
		rect(s, 0, 0, W, H, fade(colInk, a.flash*.4))
	}

	veil := func(y, h float64) { rect(s, 0, y, W, h, fade(colBg, .85)) }
	switch p.Ph {
	case PhCount:
		veil(80, 80)
		drawAvatar(s, 34, 100, 3, av1, a.tick, false)
		drawAvatar(s, 170, 100, 3, av2, a.tick+40, true)
		txt(s, av1.DisplayName(), 52, 132, 8, c1, 0)
		txt(s, av2.DisplayName(), 188, 132, 8, c2, 0)
		txt(s, strconv.Itoa(int(math.Max(1, math.Ceil(p.Tm)))), W/2+2, 100, 32, colInk, 0)
		if a.mode == ModeLocal {
			txt(s, "W / S", 18, H-18, 8, colDim, -1)
			txt(s, "FLECHAS", W-18, H-18, 8, colDim, 1)
		}
		if a.nw != nil {
			if a.nw.Direct() {
				txt(s, "DIRECTO", W/2, 146, 8, colOk, 0)
			} else {
				txt(s, "POR SERVIDOR", W/2, 146, 8, colDim, 0)
			}
		}
	case PhOver:
		w := av1
		if p.Winner == 2 {
			w = av2
		}
		veil(56, 124)
		bounce := math.Abs(math.Sin(float64(a.tick)/8)) * 6
		drawAvatar(s, 96, 84-bounce, 4, w, a.tick, false)
		t := "GANA " + w.DisplayName()
		size := 16.0
		if len(t) > 13 {
			size = 8
		}
		txt(s, t, W/2, 124, size, w.Col(), 0)
		txt(s, fmt.Sprintf("%d - %d", p.S1, p.S2), W/2, 146, 8, colInk, 0)
		txt(s, "ESPACIO = REVANCHA", W/2, 162, 8, colDim, 0)
	case PhDC:
		veil(84, 72)
		txt(s, "SE CORTO", W/2, 100, 16, colErr, 0)
		txt(s, "LA CONEXION", W/2, 124, 8, colInk, 0)
		txt(s, "ESPACIO = SALIR", W/2, 142, 8, colDim, 0)
	}
	if p.Paused && p.Ph != PhOver && p.Ph != PhDC {
		veil(84, 72)
		txt(s, "PAUSA", W/2, 104, 16, colInk, 0)
		sub := "P = SEGUIR"
		if a.mode == ModeHost || a.mode == ModeGuest {
			sub = "UN MOMENTO" + dots(a.tick)
		}
		txt(s, sub, W/2, 130, 8, colDim, 0)
	}
	for _, b := range a.topButtons() {
		a.drawIconBtn(s, b, .3)
	}
}

// ---------- ícono de la ventana (neutro: unas barras de actividad) ----------

func appIcon(size int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	k := size / 16
	fill := func(x0, y0, w, h int, c color.NRGBA) {
		for y := y0 * k; y < (y0+h)*k; y++ {
			for x := x0 * k; x < (x0+w)*k; x++ {
				out.SetNRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, 16, 16, colField)
	fill(3, 8, 2, 5, colDim)
	fill(7, 5, 2, 8, colInk)
	fill(11, 9, 2, 4, colDim)
	return out
}

// ---------- capturas para pruebas (MJ_SHOT_DIR) ----------

var shotDir = os.Getenv("MJ_SHOT_DIR")

func shot(s *ebiten.Image, tick int) {
	if shotDir == "" || tick%30 != 0 {
		return
	}
	img := image.NewRGBA(s.Bounds()) // ReadPixels entrega color premultiplicado, como image.RGBA
	s.ReadPixels(img.Pix)
	f, err := os.Create(filepath.Join(shotDir, fmt.Sprintf("f%05d.png", tick)))
	if err == nil {
		png.Encode(f, img)
		f.Close()
	}
}
