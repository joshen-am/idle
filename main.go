package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
)

// ---------- configuración guardada (personaje, sonido, tamaño, posición) ----------

type Config struct {
	Av     Avatar
	HasAv  bool
	Mute   bool
	HasCfg bool
	Room   string // sala online: los dos entran a la misma y se encuentran solos
}

func cfgPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "Idle", "config.json")
}

func loadCfg() Config {
	var c Config
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	if !c.HasCfg { // primera vez: personaje al azar y sin sonido (es la oficina)
		c.Mute, c.HasCfg = true, true
	}
	if !c.HasAv {
		c.Av, c.HasAv = randomAvatar(), true
	}
	c.Av = c.Av.fix()
	if c.Room = cleanName(c.Room); c.Room == "" {
		c.Room = "IDLE"
	}
	return c
}

func saveCfg(c Config) {
	p := cfgPath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(p, b, 0o644)
}

func wrap(v uint8, d, n int) uint8 { return uint8(((int(v)+d)%n + n) % n) }

// limitVec acorta el vector (x, y) a un largo máximo.
func limitVec(x, y, max float64) (float64, float64) {
	if d := math.Hypot(x, y); d > max {
		return x * max / d, y * max / d
	}
	return x, y
}

// advanceBall mueve la bola t segundos rebotando arriba/abajo (sin paletas), para predecir.
func advanceBall(x, y, vx, vy, t float64) (float64, float64, float64, float64) {
	x = clamp(x+vx*t, PX+PW-2, W-PX-PW-BS+2)
	y += vy * t
	for i := 0; i < 4 && (y < 0 || y > H-BS); i++ {
		if y < 0 {
			y, vy = -y, math.Abs(vy)
		} else {
			y, vy = 2*(H-BS)-y, -math.Abs(vy)
		}
	}
	return x, clamp(y, 0, H-BS), vx, vy
}

var botMode = os.Getenv("MJ_BOT") != ""

func main() {
	if err := runOverlay(); err != nil {
		log.Fatal(err)
	}
}
