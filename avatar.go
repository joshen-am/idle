package main

import (
	"image/color"
	"math"
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
	col2 color.NRGBA // gorros: color de las 'o'
	anim bool        // color que va cambiando (arcoíris)
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
	// de la tienda (2.3)
	{name: "PULPO", eyeY: 2, rows: []string{
		"...######...",
		"..########..",
		".##########.",
		".##########.",
		".##########.",
		"..########..",
		".l.l.ll.l.l.",
		"l.l..ll..l.l",
	}},
	{name: "HONGO", eyeY: 3, rows: []string{
		"..########..",
		".##########.",
		"############",
		"...######...",
		"...######...",
		"...######...",
		"...l....l...",
		"..ll....ll..",
	}},
	{name: "SLIME", eyeY: 4, rows: []string{
		"............",
		"............",
		"....####....",
		"..########..",
		".##########.",
		"############",
		"############",
		"############",
	}},
	{name: "PINGUINO", eyeY: 1, rows: []string{
		"...######...",
		"..########..",
		"..########..",
		".##########.",
		".##########.",
		"..########..",
		"...ll..ll...",
		"..lll..lll..",
	}},
	{name: "CONEJO", eyeY: 3, rows: []string{
		"...#....#...",
		"...#....#...",
		"..########..",
		"..########..",
		"..########..",
		"..########..",
		"..l.l..l.l..",
		"..ll....ll..",
	}},
	{name: "ALIEN", eyeY: 3, rows: []string{
		".l........l.",
		"..l......l..",
		"..########..",
		".##########.",
		"############",
		".##########.",
		"..l..ll..l..",
		".l...ll...l.",
	}},
	{name: "OSO", eyeY: 2, rows: []string{
		".##......##.",
		".##########.",
		"..########..",
		"..########..",
		".##########.",
		".##########.",
		"..ll....ll..",
		"..ll....ll..",
	}},
	{name: "NUBE", eyeY: 3, rows: []string{
		"............",
		"....###.....",
		"..#######...",
		".##########.",
		"############",
		"############",
		".##########.",
		"............",
	}},
	{name: "BUHO", eyeY: 2, rows: []string{
		"..#......#..",
		"..########..",
		".##########.",
		"############",
		"############",
		"..########..",
		"...l....l...",
		"..ll....ll..",
	}},
	{name: "COHETE", eyeY: 3, rows: []string{
		".....##.....",
		"....####....",
		"...######...",
		"..########..",
		"..########..",
		".##########.",
		".l.l....l.l.",
		"l...l..l...l",
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
	// de la tienda
	{name: "DORADO", col: hex(0xe8b84a)},
	{name: "NOCHE", col: hex(0x5a5f8a)},
	{name: "CORAL", col: hex(0xff8f70)},
	{name: "BOSQUE", col: hex(0x4f8a5b)},
	{name: "CEREZA", col: hex(0xc8384a)},
	{name: "TURQUESA", col: hex(0x3fc1c0)},
	{name: "LAVANDA", col: hex(0xc7b8f0)},
	{name: "CHOCOLATE", col: hex(0x7a5236)},
	{name: "NIEVE", col: hex(0xeeeeea)},
	{name: "CARBON", col: hex(0x3c3c40)},
	{name: "MOSTAZA", col: hex(0xd8a530)},
	{name: "CHICLE", col: hex(0xff8fd0)},
	{name: "OLIVA", col: hex(0x8a8f3c)},
	{name: "CIRUELA", col: hex(0x7a3f7f)},
	{name: "ARCOIRIS", col: hex(0xd98aa6), anim: true},
}

var eyeNames = []string{"PUNTOS", "GRANDES", "FELICES", "DORMIDO", "PICARO", "LENTES",
	"ENOJADO", "TRISTE", "CURIOSO", "MINI", "ANIME", "LASER", "3D", "VISOR", "CICLOPE", "ESTRELLAS"}

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
	// de la tienda
	{name: "FIESTA", col: hex(0xd96fb8), rows: []string{
		"......#.....",
		".....###....",
		"....#####...",
	}},
	{name: "VIKINGO", col: hex(0xb8b8c0), rows: []string{
		".#........#.",
		".##.####.##.",
		"..########..",
	}},
	{name: "AURA", col: hex(0xf2e08a), rows: []string{
		"...######...",
		"..#......#..",
		"...######...",
	}},
	{name: "CHEF", col: hex(0xf0f0f0), rows: []string{
		"..##.##.##..",
		"..########..",
		"...######...",
	}},
	// de la tienda (2.3); las 'o' van en el segundo color
	{name: "VAQUERO", col: hex(0x9a6a3a), col2: hex(0x5a3a1a), rows: []string{
		"....####....",
		"...#oooo#...",
		"############",
	}},
	{name: "LANA", col: hex(0xc8384a), col2: hex(0xf0f0f0), rows: []string{
		".....oo.....",
		"...######...",
		"..oooooooo..",
	}},
	{name: "MAGO", col: hex(0x5a3f9a), col2: hex(0xf2c94c), rows: []string{
		"......##....",
		".....#o##...",
		"..########..",
	}},
	{name: "CUERNOS", col: hex(0xd8403a), rows: []string{
		"..#......#..",
		"..##....##..",
		"............",
	}},
	{name: "PIRATA", col: hex(0x2a2a2a), col2: hex(0xf0f0f0), rows: []string{
		"...######...",
		"..###oo###..",
		"############",
	}},
	{name: "ANTENAS", col: hex(0x3a3a3a), col2: hex(0xff8fd0), rows: []string{
		"..o......o..",
		"...#....#...",
		"....#..#....",
	}},
	{name: "PUNK", col: hex(0xff4fa0), rows: []string{
		".....##.....",
		".....##.....",
		"....####....",
	}},
	{name: "NAVIDAD", col: hex(0xc8384a), col2: hex(0xf0f0f0), rows: []string{
		"......###.o.",
		"....####..#.",
		"..oooooooo..",
	}},
	{name: "PAJARO", col: hex(0xf2c94c), col2: hex(0xe8803a), rows: []string{
		"....o##.....",
		"....####....",
		"....#..#....",
	}},
	{name: "BIRRETE", col: hex(0x2a2a2a), col2: hex(0xe8b84a), rows: []string{
		"............",
		"############",
		".o.######...",
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

// rainbow: color del arcoíris en el momento t.
func rainbow(t int) color.NRGBA {
	h := float64(t%360) / 60
	x := 1 - math.Abs(math.Mod(h, 2)-1)
	r, g, b := 0.0, 0.0, 0.0
	switch int(h) {
	case 0:
		r, g = 1, x
	case 1:
		r, g = x, 1
	case 2:
		g, b = 1, x
	case 3:
		g, b = x, 1
	case 4:
		r, b = x, 1
	default:
		r, b = 1, x
	}
	m := func(v float64) uint8 { return uint8(110 + v*120) }
	return color.NRGBA{m(r), m(g), m(b), 255}
}

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

// Pose: cómo está el personaje en este momento (para animarlo como mascota).
type Pose struct {
	Still  bool    // sin respirar ni parpadear (animaciones apagadas)
	Eyes   int     // -1 = sus ojos; 0..5 = un estilo (3 = cerrados, 1 = sorprendido, 2 = feliz)
	EyeDX  int     // mira hacia un lado (-1 izquierda, 1 derecha)
	Mouth  bool    // boca abierta (bostezo o sorpresa)
	Dangle bool    // lo levantaron: las patitas cuelgan y se mueven
	SX, SY float64 // estirar/aplastar (1 = normal)
}

var normalPose = Pose{Eyes: -1, SX: 1, SY: 1}

// drawAvatarTint dibuja el personaje; con tint todo sale de ese color (sirve para la sombra).
func drawAvatarTint(dst *ebiten.Image, x, y, sc float64, av Avatar, t int, flip bool, tint color.Color) {
	drawAvatarPose(dst, x, y, sc, av, t, flip, tint, normalPose)
}

// drawAvatarPose dibuja el personaje en una pose. (x, y) es la esquina del cuerpo de 12x8;
// al estirar o aplastar, los pies quedan en su lugar.
func drawAvatarPose(dst *ebiten.Image, x, y, sc float64, av Avatar, t int, flip bool, tint color.Color, pose Pose) {
	av = av.fix()
	b := bodies[av.Body]
	c := av.Col()
	if avColors[av.Color].anim {
		c = rainbow(t * 2)
	}
	if pose.SX == 0 {
		pose.SX = 1
	}
	if pose.SY == 0 {
		pose.SY = 1
	}
	if !pose.Still && (t/30)%2 == 0 {
		y -= sc // respira
	}
	cw, ch := sc*pose.SX, sc*pose.SY
	cell := func(cx, cy int, col color.Color) {
		if flip {
			cx = 11 - cx
		}
		if tint != nil {
			col = tint
		}
		rect(dst, x+6*sc-6*cw+float64(cx)*cw, y+8*sc-float64(8-cy)*ch, cw, ch, col)
	}
	for r, row := range b.rows {
		for i, chr := range row {
			switch chr {
			case '#':
				cell(i, r, c)
			case 'l':
				if pose.Dangle && r >= 5 { // patitas colgando: más largas y moviéndose
					dx := 0
					if (t/6+i)%2 == 0 {
						dx = 1
					}
					cell(i+dx-(i/6)*dx*2, r, shade(c, .72))
					cell(i+dx-(i/6)*dx*2, r+1, shade(c, .72))
				} else {
					cell(i, r, shade(c, .72))
				}
			}
		}
	}
	// ojos (parpadean cada tanto)
	ink := colBg
	if int(c.R)*3+int(c.G)*6+int(c.B) < 900 { // cuerpo muy oscuro: ojos claros
		ink = colInk
	}
	ey := b.eyeY
	style := int(av.Eyes)
	if pose.Eyes >= 0 {
		style = pose.Eyes
	}
	if !pose.Still && t%200 < 7 && style != 5 && style != 12 && style != 13 {
		style = 3
	}
	red, blue, gold, white := hex(0xe0404a), hex(0x4a8ae0), hex(0xf2c94c), hex(0xf4f4f0)
	for side, ex := range []int{3 + pose.EyeDX, 7 + pose.EyeDX} {
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
		case 6: // enojado: el lado de adentro más bajo
			if side == 0 {
				on = [][2]int{{0, 0}, {0, 1}, {1, 1}}
			} else {
				on = [][2]int{{1, 0}, {0, 1}, {1, 1}}
			}
		case 7: // triste
			if side == 0 {
				on = [][2]int{{1, 0}, {0, 1}, {1, 1}}
			} else {
				on = [][2]int{{0, 0}, {0, 1}, {1, 1}}
			}
		case 8: // curioso: mira arriba
			on = [][2]int{{0, 0}, {1, 0}}
		case 9: // mini
			if side == 0 {
				on = [][2]int{{1, 0}}
			} else {
				on = [][2]int{{0, 0}}
			}
		case 10: // anime: con brillo
			on = [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
			for _, o := range on {
				cell(ex+o[0], ey+o[1], ink)
			}
			cell(ex, ey, white)
			on = nil
		case 11: // láser
			for _, o := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				cell(ex+o[0], ey+o[1], red)
			}
		case 12: // lentes 3D
			col := red
			if side == 1 {
				col = blue
			}
			for _, o := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				cell(ex+o[0], ey+o[1], col)
			}
		case 14: // cíclope: un solo ojo grande al medio
			if side == 0 {
				for _, o := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
					cell(5+pose.EyeDX+o[0], ey+o[1], white)
				}
				cell(5+pose.EyeDX+1, ey+1, ink)
			}
		case 15: // estrellas
			for _, o := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				cell(ex+o[0], ey+o[1], gold)
			}
			cell(ex+side, ey+side, white)
		}
		for _, o := range on {
			cell(ex+o[0], ey+o[1], ink)
		}
	}
	if style == 12 {
		for i := 5; i <= 6; i++ {
			cell(i+pose.EyeDX, ey, ink)
		}
	}
	if style == 13 { // visor
		for i := 2; i <= 9; i++ {
			cell(i, ey, hex(0x3fc1c0))
			cell(i, ey+1, hex(0x2a8a8a))
		}
		cell(3+pose.EyeDX+(t/20)%6, ey, white)
	}
	if style == 5 {
		for i := 2; i <= 9; i++ {
			cell(i+pose.EyeDX, ey, ink)
		}
	}
	if pose.Mouth {
		cell(5, ey+3, ink)
		cell(6, ey+3, ink)
	}
	if h := hats[av.Hat]; h.rows != nil {
		for r, row := range h.rows {
			for i, chr := range row {
				switch chr {
				case '#':
					cell(i, r-3, h.col)
				case 'o':
					cell(i, r-3, h.col2)
				}
			}
		}
	}
}
