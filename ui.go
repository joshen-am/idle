package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

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

func dots(t int) string { return strings.Repeat(".", (t/18)%4) }

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
