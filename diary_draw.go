package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/colorm"
)

// El diario se dibuja como un periódico viejo: papel, tinta y "fotos" en sepia.

var (
	paper    = hex(0xece2c8)
	paperDk  = hex(0xd6c8a6)
	inkP     = hex(0x2b2419)
	inkP2    = hex(0x6b5d44)
	inkRed   = hex(0x8e3324)
	photoBuf = map[[2]int]*ebiten.Image{}
)

const diaryW, diaryH = 560.0, 444.0

func (o *Overlay) drawDiary(s *ebiten.Image) {
	if o.diary == nil {
		o.diary = buildDiary(&o.cfg, time.Now())
	}
	d := o.diary
	w, h := diaryW, diaryH
	x, y := o.panel(s, w, h)
	rect(s, x, y, w, h, paper)
	frame(s, x, y, w, h, paperDk)
	r := rand.New(rand.NewSource(int64(d.No))) // manchitas del papel
	for i := 0; i < 70; i++ {
		rect(s, x+4+r.Float64()*(w-8), y+4+r.Float64()*(h-8), 1+float64(r.Intn(2)), 1, color.NRGBA{110, 90, 50, 40})
	}
	if len(d.Pages) == 0 {
		return
	}
	o.diaryPage = clampInt(o.diaryPage, 0, len(d.Pages)-1)
	pg := d.Pages[o.diaryPage]
	L, W := x+20, w-40
	var cy float64
	if o.diaryPage == 0 { // cabecera grande
		txt(s, "EL DIARIO IDLE", x+w/2, y+14, 24, inkP, 0)
		rect(s, L, y+46, W, 2, inkP)
		rect(s, L, y+50, W, 1, inkP)
		txt(s, fmt.Sprintf("N.%d · %s · PRECIO: 1 ¤", d.No, d.Date), x+w/2, y+57, 8, inkP2, 0)
		rect(s, L, y+70, W, 1, inkP)
		cy = y + 80
	} else {
		txt(s, "EL DIARIO IDLE", L, y+12, 8, inkP2, -1)
		txt(s, fmt.Sprintf("N.%d · PAG %d", d.No, o.diaryPage+1), L+W, y+12, 8, inkP2, 1)
		txt(s, pg.Section, x+w/2, y+28, 16, inkP, 0)
		rect(s, L, y+50, W, 2, inkP)
		rect(s, L, y+54, W, 1, inkP)
		cy = y + 64
	}
	bottom := y + h - 46
	t := o.tick

	switch {
	case pg.Head != "": // portada
		for _, l := range limitLines(wrapText(pg.Head, int(W/16)), 2) {
			txt(s, l, x+w/2, cy, 16, inkP, 0)
			cy += 22
		}
		cy += 4
		if a := pg.Arts[0]; a.Photo != nil {
			drawPhoto(s, a.Photo, L, cy, W, 150, t)
			cy += 158
		}
		for _, l := range limitLines(wrapText(pg.Arts[0].Text, int(W/8)), 5) {
			txt(s, l, L, cy, 8, inkP, -1)
			cy += 13
		}
		if o.tipToday {
			txt(s, "+10 ¤ POR LEER EL DIARIO", x+w/2, bottom-6, 8, inkRed, 0)
		}
	case len(pg.Results) > 0: // deportes
		txt(s, "RESULTADOS DE AYER", L, cy, 8, inkRed, -1)
		cy += 16
		for _, rs := range pg.Results {
			drawAvatar(s, L, cy+1, 1.5, rs.WAv, t, false)
			txt(s, rs.W, L+26, cy+2, 8, inkP, -1)
			txt(s, fmt.Sprintf("%d-%d", rs.SW, rs.SL), x+w/2, cy+2, 8, inkP, 0)
			txt(s, rs.L, L+W-26, cy+2, 8, inkP2, 1)
			drawAvatar(s, L+W-18, cy+1, 1.5, rs.LAv, t, true)
			rect(s, L, cy+17, W, 1, paperDk)
			cy += 22
		}
		if len(pg.Arts) > 0 && bottom-cy > 100 {
			cy += 6
			a := pg.Arts[0]
			ph := minF(110, bottom-cy)
			drawPhoto(s, a.Photo, L, cy, 200, ph, t)
			txt(s, a.Title, L+214, cy+4, 8, inkRed, -1)
			ly := cy + 22
			for _, l := range limitLines(wrapText("\""+a.Text+"\"", int((W-214)/8)), int((ph-22)/13)) {
				txt(s, l, L+214, ly, 8, inkP, -1)
				ly += 13
			}
		}
	case pg.Ad != "": // ranking y clasificados
		a := pg.Arts[0]
		drawPhoto(s, a.Photo, L, cy, W, 130, t)
		cy += 140
		for _, l := range limitLines(wrapText(a.Text, int(W/8)), 4) {
			txt(s, l, L, cy, 8, inkP, -1)
			cy += 13
		}
		cy += 8
		bh := bottom - cy
		if bh > 50 {
			frame(s, L, cy, W, bh, inkP)
			txt(s, "CLASIFICADOS", x+w/2, cy+8, 8, inkRed, 0)
			ly := cy + 24
			for i, ad := range strings.Split(pg.Ad, "\n") {
				if i > 0 {
					rect(s, x+w/2-40, ly-2, 80, 1, inkP2)
					ly += 8
				}
				for _, l := range limitLines(wrapText(ad, int((W-20)/8)), 3) {
					if ly+13 < cy+bh {
						txt(s, l, x+w/2, ly, 8, inkP, 0)
					}
					ly += 13
				}
			}
		}
	default: // artículos con foto al lado
		n := len(pg.Arts)
		rowH, phW := 104.0, 170.0
		if n > 3 {
			rowH, phW = (bottom-cy)/float64(n), 96
		}
		for i, a := range pg.Arts {
			ry := cy + float64(i)*rowH
			drawPhoto(s, a.Photo, L, ry, phW, rowH-10, t+i*37)
			tx := L + phW + 12
			txt(s, a.Title, tx, ry+2, 8, inkRed, -1)
			ly := ry + 18
			for _, l := range limitLines(wrapText(a.Text, int((L+W-tx)/8)), int((rowH-28)/13)) {
				txt(s, l, tx, ly, 8, inkP, -1)
				ly += 13
			}
			if i < n-1 {
				rect(s, L, ry+rowH-5, W, 1, paperDk)
			}
		}
	}

	// pie: páginas y cerrar
	rect(s, L, y+h-40, W, 1, inkP)
	if len(d.Pages) > 1 {
		b := o.button("dy:prev", L-6, y+h-36, 34, 28)
		c := inkP
		if o.diaryPage == 0 {
			c = paperDk
		}
		txt(s, "◀", b.x+b.w/2, b.y+7, 16, c, 0)
		txt(s, fmt.Sprintf("PAG %d DE %d", o.diaryPage+1, len(d.Pages)), L+90, y+h-26, 8, inkP2, 0)
		b = o.button("dy:next", L+146, y+h-36, 34, 28)
		c = inkP
		if o.diaryPage == len(d.Pages)-1 {
			c = paperDk
		}
		txt(s, "▶", b.x+b.w/2, b.y+7, 16, c, 0)
	}
	b := o.button("cerrar", L+W-100, y+h-36, 100, 28)
	frame(s, b.x, b.y, b.w, b.h, inkP)
	txt(s, "CERRAR", b.x+b.w/2, b.y+7, 16, inkP, 0)
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func limitLines(l []string, n int) []string {
	if n < 1 {
		return nil
	}
	if len(l) > n {
		l = l[:n]
		l[n-1] += "..."
	}
	return l
}

// drawPhoto arma la escena en una imagen aparte y la pega en sepia, con borde y trama.
func drawPhoto(s *ebiten.Image, ph *Photo, x, y, w, h float64, t int) {
	if ph == nil || w < 10 || h < 10 {
		return
	}
	key := [2]int{int(w), int(h)}
	img := photoBuf[key]
	if img == nil {
		img = ebiten.NewImage(key[0], key[1])
		photoBuf[key] = img
	}
	img.Clear()
	ox, oy := drawOX, drawOY
	drawOX, drawOY = 0, 0
	composePhoto(img, ph, w, h, t)
	drawOX, drawOY = ox, oy

	var cm colorm.ColorM
	cm.ChangeHSV(0, 0, 1)
	cm.Scale(1.0, .9, .72, 1)
	op := &colorm.DrawImageOptions{}
	op.GeoM.Translate(x+drawOX, y+drawOY)
	colorm.DrawImage(s, img, cm, op)
	for yy := 1.0; yy < h; yy += 3 { // trama de imprenta
		rect(s, x, y+yy, w, 1, color.NRGBA{60, 45, 20, 26})
	}
	frame(s, x, y, w, h, inkP)
}

func composePhoto(img *ebiten.Image, ph *Photo, w, h float64, t int) {
	rect(img, 0, 0, w, h*.7, hex(0xd8d8d0))
	rect(img, 0, h*.7, w, h*.3, hex(0x9a968c))
	sc := float64(int(h / 17))
	if sc < 1.5 {
		sc = 1.5
	}
	fy := h*.7 + sc*1.5 // los pies
	av := func(i int) Avatar {
		if i < len(ph.Avs) {
			return ph.Avs[i]
		}
		return avAny
	}
	put := func(a Avatar, cx, k float64, eyes int, flip bool, lift float64) {
		p := normalPose
		p.Eyes = eyes
		s := sc * k
		drawAvatarPose(img, cx-6*s, fy-8*s-lift, s, a, t, flip, nil, p)
	}
	label := func(str string, cx, cy, size float64) { txt(img, str, cx, cy, size, hex(0x202020), 0) }
	switch ph.Kind {
	case "win":
		lift := 0.0
		if (t/20)%2 == 0 {
			lift = sc
		}
		put(av(0), w*.36, 1.25, 2, false, lift)
		put(av(1), w*.68, .9, 7, true, 0)
		label("★", w*.36-sc*9, fy-sc*13, sc*3)
		label("★", w*.36+sc*8, fy-sc*15, sc*2.5)
		rect(img, w*.68-sc*1.5, fy-sc*3, sc*.8, sc*1.4, hex(0xf8f8f8)) // lágrima
	case "duo", "peace":
		put(av(0), w*.36, 1, 2, false, 0)
		put(av(1), w*.64, 1, 2, true, 0)
		sym := "♥"
		if ph.Kind == "peace" {
			sym = "♪"
		}
		label(sym, w/2, fy-sc*15, sc*3)
	case "prank":
		put(av(0), w*.34, 1, 4, false, 0)
		put(av(1), w*.66, 1, 1, true, sc)
		label("!", w*.66, fy-sc*17, sc*3)
	case "fight":
		put(av(0), w*.36, 1, 6, false, 0)
		put(av(1), w*.64, 1, 6, true, 0)
		label("#@!", w/2, fy-sc*16, sc*2.2)
	case "visit":
		rect(img, w*.56, fy-sc*10, sc*9, sc*7, hex(0x303030)) // pantalla
		rect(img, w*.56+sc, fy-sc*9, sc*7, sc*5, hex(0xb8c8c8))
		rect(img, w*.56+sc*4, fy-sc*3, sc, sc*3, hex(0x303030))
		put(av(0), w*.32, 1, 8, false, 0)
		label("?", w*.32, fy-sc*15, sc*3)
	case "podium":
		bw := sc * 16
		for i, hgt := range []float64{1.9, 1.3, .9} {
			cx := []float64{w / 2, w/2 - bw*1.05, w/2 + bw*1.05}[i]
			bh := sc * 3 * hgt
			rect(img, cx-bw/2, h-bh, bw, bh, hex(0x707070))
			label(fmt.Sprint(i+1), cx, h-bh+sc, sc*2.4)
			if i < len(ph.Avs) {
				drawAvatarPose(img, cx-6*sc, h-bh-8*sc, sc, ph.Avs[i], t+i*20, false, nil, Pose{Eyes: []int{2, -1, 7}[i], SX: 1, SY: 1})
			}
		}
	case "cast":
		for i, cp := range cpus {
			put(cp.Av, w*(float64(i)+.5)/float64(len(cpus)), .9, -1, i%2 == 1, 0)
		}
	default: // retrato
		eyes := 2
		switch ph.Tag {
		case "sleep":
			eyes = 3
			label("Z", w/2+sc*9, fy-sc*15, sc*2.5)
			label("z", w/2+sc*13, fy-sc*19, sc*1.8)
		case "smug":
			eyes = 4
		case "angry":
			eyes = 6
		case "fast":
			for i := 0; i < 4; i++ {
				rect(img, w/2-sc*16, fy-sc*7+float64(i)*sc*1.6, sc*(6-float64(i)), sc*.5, hex(0x707070))
			}
		}
		put(av(0), w/2, 1.2, eyes, false, 0)
		if ph.Tag == "fade" { // Bit: medio transparente
			rect(img, 0, 0, w, h*.7, color.NRGBA{216, 216, 208, 120})
		}
	}
}
