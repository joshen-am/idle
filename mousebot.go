package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Mouse de prueba (MJ_MOUSE): una lista de puntos "segundo:x:y:botón" que se recorre
// interpolando, para probar clics y arrastres sin una persona. Ej.: "1:100:100:0 2:100:100:1".
// Las coordenadas negativas cuentan desde la derecha/abajo de la pantalla.

type mousePoint struct {
	t, x, y float64
	down    bool
}

func init() {
	var pts []mousePoint
	for _, tok := range strings.Fields(os.Getenv("MJ_MOUSE")) {
		f := strings.Split(tok, ":")
		if len(f) != 4 {
			continue
		}
		t, _ := strconv.ParseFloat(f[0], 64)
		x, _ := strconv.ParseFloat(f[1], 64)
		y, _ := strconv.ParseFloat(f[2], 64)
		pts = append(pts, mousePoint{t, x, y, f[3] == "1"})
	}
	if len(pts) == 0 {
		return
	}
	start := time.Now()
	botMouse = func(o *Overlay) (float64, float64, bool) {
		t := time.Since(start).Seconds()
		abs := func(v, size float64) float64 {
			if v < 0 {
				return size + v
			}
			return v
		}
		sw, sh := float64(o.sw), float64(o.sh)
		prev := pts[0]
		for _, p := range pts {
			if p.t > t {
				k := 0.0
				if p.t > prev.t {
					k = clamp((t-prev.t)/(p.t-prev.t), 0, 1)
				}
				x := abs(prev.x, sw) + (abs(p.x, sw)-abs(prev.x, sw))*k
				y := abs(prev.y, sh) + (abs(p.y, sh)-abs(prev.y, sh))*k
				return x, y, prev.down
			}
			prev = p
		}
		return abs(prev.x, sw), abs(prev.y, sh), prev.down
	}
}
