package main

import (
	"image/color"
	"math/rand"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Personajes pixelados de 12x8 (más 3 filas arriba para el gorro). Cada uno elige el suyo.

type Avatar struct {
	Body, Color, Eyes, Hat uint8
	Name                   string
}

type part struct {
	name string
	rows []string
	eyeY int // fila donde van los ojos (solo cuerpos)
	col  color.NRGBA
}

var bodies = []part{
	{name: "BLOQUE", eyeY: 1, rows: []string{
		"..########..",
		"..########..",
		"############",
		"############",
		"..########..",
		"..########..",
		"..l.l..l.l..",
		"..l.l..l.l..",
	}},
	{name: "BOLITA", eyeY: 2, rows: []string{
		"...######...",
		"..########..",
		".##########.",
		".##########.",
		".##########.",
		"..########..",
		"...l....l...",
		"..ll....ll..",
	}},
	{name: "FANTASMA", eyeY: 2, rows: []string{
		"...######...",
		"..########..",
		".##########.",
		".##########.",
		".##########.",
		".##########.",
		".##.####.##.",
		".#...##...#.",
	}},
	{name: "CANGREJO", eyeY: 3, rows: []string{
		"l..........l",
		"ll........ll",
		".l.######.l.",
		"..########..",
		"..########..",
		"..########..",
		"..l.l..l.l..",
		".l..l..l..l.",
	}},
	{name: "ROBOT", eyeY: 3, rows: []string{
		".....l......",
		".....l......",
		"..########..",
		"..########..",
		"l.########.l",
		"l.########.l",
		"...ll..ll...",
		"...ll..ll...",
	}},
	{name: "GATO", eyeY: 3, rows: []string{
		"..#......#..",
		"..##....##..",
		"..########..",
		"..########..",
		".##########.",
		"..########..",
		"..l.l..l.l..",
		"..l.l..l.l..",
	}},
}

var avColors = []part{
	{name: "NARANJA", col: hex(0xd97757)},
	{name: "ARENA", col: hex(0xd4b896)},
	{name: "MENTA", col: hex(0x7fb8a4)},
	{name: "CIELO", col: hex(0x6fa3c7)},
	{name: "LILA", col: hex(0xa58fc9)},
	{name: "ROSA", col: hex(0xd98aa6)},
	{name: "LIMON", col: hex(0xc9c25c)},
	{name: "GRIS", col: hex(0x9a9690)},
}

var eyeNames = []string{"PUNTOS", "GRANDES", "FELICES", "DORMIDO", "PICARO", "LENTES"}

var hats = []part{
	{name: "NADA"},
	{name: "GORRA", col: hex(0x4a6fa5), rows: []string{
		"............",
		"...######...",
		"...########.",
	}},
	{name: "COPA", col: hex(0x5a5650), rows: []string{
		"...######...",
		"...######...",
		".##########.",
	}},
	{name: "CORONA", col: hex(0xd9b45c), rows: []string{
		"............",
		"..#..##..#..",
		"..########..",
	}},
	{name: "BROTE", col: hex(0x7fb87a), rows: []string{
		"....##.##...",
		".....###....",
		"......#.....",
	}},
	{name: "LAZO", col: hex(0xd98aa6), rows: []string{
		"............",
		"...##..##...",
		"....####....",
	}},
	{name: "AUDIFONOS", col: hex(0x9a9690), rows: []string{
		"....####....",
		"...#....#...",
		"..#......#..",
	}},
}

var funNames = []string{"BYTE", "PIXEL", "NUBE", "CHISPA", "TOFU", "MOCHI", "COMETA", "BIT"}

func randomAvatar() Avatar {
	return Avatar{Body: uint8(rand.Intn(len(bodies))), Color: uint8(rand.Intn(len(avColors))),
		Eyes: uint8(rand.Intn(3)), Name: funNames[rand.Intn(len(funNames))]}
}

var (
	avP2  = Avatar{Body: 1, Color: 3, Eyes: 2, Name: "P2"}
	avCPU = Avatar{Body: 4, Color: 7, Eyes: 0, Hat: 6, Name: "CPU"}
	avAny = Avatar{Body: 2, Color: 7, Eyes: 3, Name: "..."}
)

func (av Avatar) fix() Avatar {
	av.Body %= uint8(len(bodies))
	av.Color %= uint8(len(avColors))
	av.Eyes %= uint8(len(eyeNames))
	av.Hat %= uint8(len(hats))
	av.Name = cleanName(av.Name)
	return av
}

func cleanName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') && b.Len() < 6 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (av Avatar) Col() color.NRGBA { return avColors[int(av.Color)%len(avColors)].col }

func (av Avatar) DisplayName() string {
	if av.Name == "" {
		return "?"
	}
	return av.Name
}

// Para mandarlo por la red: 4 bytes + nombre.
func (av Avatar) Encode() []byte {
	return append([]byte{av.Body, av.Color, av.Eyes, av.Hat}, []byte(cleanName(av.Name))...)
}

func DecodeAvatar(b []byte) (Avatar, bool) {
	if len(b) < 4 {
		return Avatar{}, false
	}
	return Avatar{Body: b[0], Color: b[1], Eyes: b[2], Hat: b[3], Name: string(b[4:])}.fix(), true
}

func shade(c color.NRGBA, k float64) color.NRGBA {
	return color.NRGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), c.A}
}

// drawAvatar dibuja el personaje con la esquina superior izquierda del cuerpo en (x, y).
// t anima (respira y parpadea); flip lo da vuelta para que mire a la izquierda.
func drawAvatar(dst *ebiten.Image, x, y, sc float64, av Avatar, t int, flip bool) {
	drawAvatarTint(dst, x, y, sc, av, t, flip, nil)
}

// drawAvatarTint dibuja el personaje; con tint todo sale de ese color (sirve para la sombra).
func drawAvatarTint(dst *ebiten.Image, x, y, sc float64, av Avatar, t int, flip bool, tint color.Color) {
	av = av.fix()
	b := bodies[av.Body]
	c := av.Col()
	if (t/30)%2 == 0 {
		y -= sc // respira
	}
	cell := func(cx, cy int, col color.Color) {
		if flip {
			cx = 11 - cx
		}
		if tint != nil {
			col = tint
		}
		rect(dst, x+float64(cx)*sc, y+float64(cy)*sc, sc, sc, col)
	}
	for r, row := range b.rows {
		for i, ch := range row {
			switch ch {
			case '#':
				cell(i, r, c)
			case 'l':
				cell(i, r, shade(c, .72))
			}
		}
	}
	// ojos (parpadean cada tanto)
	ink := colBg
	ey := b.eyeY
	style := int(av.Eyes)
	if t%200 < 7 && style != 5 {
		style = 3
	}
	for side, ex := range []int{3, 7} {
		var on [][2]int
		switch style {
		case 0: // puntos
			if side == 0 {
				on = [][2]int{{1, 0}, {1, 1}}
			} else {
				on = [][2]int{{0, 0}, {0, 1}}
			}
		case 1: // grandes
			on = [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
		case 2: // felices
			if side == 0 {
				on = [][2]int{{0, 1}, {1, 0}}
			} else {
				on = [][2]int{{0, 0}, {1, 1}}
			}
		case 3: // sueño
			on = [][2]int{{0, 1}, {1, 1}}
		case 4: // guiño
			if side == 0 {
				on = [][2]int{{1, 0}, {1, 1}}
			} else {
				on = [][2]int{{0, 1}, {1, 1}}
			}
		case 5: // lentes
			on = [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
		}
		for _, o := range on {
			cell(ex+o[0], ey+o[1], ink)
		}
	}
	if style == 5 {
		for i := 2; i <= 9; i++ {
			cell(i, ey, ink)
		}
	}
	if h := hats[av.Hat]; h.rows != nil {
		for r, row := range h.rows {
			for i, ch := range row {
				if ch == '#' {
					cell(i, r-3, h.col)
				}
			}
		}
	}
}
