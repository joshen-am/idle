package main

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Teclado de todo el sistema. Cada sistema (hook_windows.go, hook_darwin.go) le pasa a onKey
// cada tecla antes de que llegue a las apps:
//   - Modo trabajo: las teclas siguen a tu app como siempre.
//   - Modo juego: las teclas son del juego y la app no las recibe.
// Para cambiar de modo: ↑ y ↓ a la vez, dos veces seguidas.
// Con ⌘/Ctrl/Alt apretado la tecla siempre pasa (los atajos del sistema siguen funcionando).

type Key int

// Letras y números usan su propio carácter ('A'..'Z', '0'..'9').
const (
	KOther Key = 1000 + iota // cualquier otra tecla que escribe algo
	KUp
	KDown
	KLeft
	KRight
	KEnter
	KSpace
	KEsc
	KBack
	KTab
)

// Estado del teclado de todo el sistema.
const (
	HookOff        = iota // este sistema no lo permite (se usa el teclado de la ventana)
	HookStarting          // arrancando
	HookOK                // funcionando
	HookPermission        // macOS: falta el permiso de Accesibilidad
	HookFailed            // no se pudo instalar
)

const chordGap = 800 * time.Millisecond // tiempo máximo entre los dos ↑+↓

type globalInput struct {
	mu        sync.Mutex
	down      map[Key]bool
	swallowed map[Key]bool // teclas cuyo "bajar" se tragó: su "soltar" también
	queue     []Key        // teclas apretadas (en modo juego) desde el último cuadro
	game      bool
	lastChord time.Time
	status    int
	toggles   int // cuántas veces se cambió de modo (para pruebas)
	swallows  int // cuántas teclas se tragaron (para pruebas)
}

var gin = &globalInput{down: map[Key]bool{}, swallowed: map[Key]bool{}}

// onKey recibe una tecla del sistema y devuelve true si hay que tragarla (no llega a la app).
// mod = hay ⌘/Ctrl/Alt apretado.
func (g *globalInput) onKey(k Key, down, mod bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !down {
		g.down[k] = false
		if g.swallowed[k] {
			delete(g.swallowed, k)
			return true
		}
		return false
	}
	wasDown := g.down[k] // si ya estaba abajo, es la repetición automática
	g.down[k] = true
	wasGame := g.game
	if !wasDown {
		toggled := false
		if k == KUp && g.down[KDown] || k == KDown && g.down[KUp] {
			if time.Since(g.lastChord) < chordGap {
				g.game, g.lastChord, toggled = !g.game, time.Time{}, true
				g.toggles++
				g.queue = g.queue[:0] // lo del gesto no le llega al juego
				logf("teclado: modo %s", map[bool]string{true: "juego", false: "trabajo"}[g.game])
			} else {
				g.lastChord = time.Now()
			}
		}
		if g.game && !mod && !toggled {
			g.queue = append(g.queue, k)
		}
	}
	if mod || !wasGame && !g.game {
		return false
	}
	g.swallowed[k] = true // la tecla del gesto también se traga, al entrar y al salir
	g.swallows++
	return true
}

// Frame entrega las teclas apretadas desde el último cuadro y si estamos en modo juego.
func (g *globalInput) Frame() ([]Key, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	q := append([]Key(nil), g.queue...)
	g.queue = g.queue[:0]
	return q, g.game
}

// Peek dice si estamos en modo juego, sin consumir las teclas pendientes.
func (g *globalInput) Peek() (int, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.queue), g.game
}

func (g *globalInput) Down(k Key) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.down[k]
}

func (g *globalInput) SetGame(on bool) {
	g.mu.Lock()
	g.game = on
	g.queue = g.queue[:0]
	g.mu.Unlock()
}

func (g *globalInput) setStatus(s int) {
	g.mu.Lock()
	g.status = s
	g.mu.Unlock()
}

func (g *globalInput) Status() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status
}

// ---------- sin teclado de sistema: se usa el de la ventana (solo si tiene el foco) ----------

var ebitenKeys = map[ebiten.Key]Key{
	ebiten.KeyArrowUp: KUp, ebiten.KeyArrowDown: KDown, ebiten.KeyArrowLeft: KLeft, ebiten.KeyArrowRight: KRight,
	ebiten.KeyEnter: KEnter, ebiten.KeySpace: KSpace, ebiten.KeyEscape: KEsc, ebiten.KeyBackspace: KBack, ebiten.KeyTab: KTab,
}

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		ebitenKeys[ebiten.KeyA+ebiten.Key(c-'A')] = Key(c)
	}
	for c := '0'; c <= '9'; c++ {
		ebitenKeys[ebiten.KeyDigit0+ebiten.Key(c-'0')] = Key(c)
	}
}

var ebitenPrev = map[ebiten.Key]bool{}

// pollWindowKeys alimenta onKey con el teclado de la ventana cuando no hay teclado de sistema.
func pollWindowKeys() {
	for ek, k := range ebitenKeys {
		d := ebiten.IsKeyPressed(ek)
		if d != ebitenPrev[ek] {
			gin.onKey(k, d, false)
			ebitenPrev[ek] = d
		}
	}
}

// ---------- guion de teclas para pruebas automáticas (MJ_SCRIPT) ----------
// Ej.: "1:UD 1.3:UD 2:Enter 3:Up+ 4:Up-" = a 1 s ↑+↓, a 1.3 s ↑+↓, a 2 s Enter,
// a 3 s aprieta ↑ y a 4 s la suelta.

type scriptStep struct {
	at   float64
	what string
}

func loadScript() []scriptStep {
	var out []scriptStep
	for _, tok := range strings.Fields(os.Getenv("MJ_SCRIPT")) {
		t, what, ok := strings.Cut(tok, ":")
		if f, err := strconv.ParseFloat(t, 64); ok && err == nil {
			out = append(out, scriptStep{f, what})
		}
	}
	return out
}

var scriptNames = map[string]Key{"Up": KUp, "Down": KDown, "Left": KLeft, "Right": KRight,
	"Enter": KEnter, "Space": KSpace, "Esc": KEsc, "Back": KBack}

func scriptKey(name string) Key {
	if k, ok := scriptNames[name]; ok {
		return k
	}
	if len(name) == 1 {
		return Key(strings.ToUpper(name)[0])
	}
	return KOther
}

// runScriptStep ejecuta un paso del guion. Devuelve las teclas a soltar en el próximo cuadro.
func runScriptStep(what string) []Key {
	switch {
	case what == "UD":
		gin.onKey(KUp, true, false)
		gin.onKey(KDown, true, false)
		return []Key{KUp, KDown}
	case strings.HasSuffix(what, "+"):
		gin.onKey(scriptKey(strings.TrimSuffix(what, "+")), true, false)
	case strings.HasSuffix(what, "-"):
		gin.onKey(scriptKey(strings.TrimSuffix(what, "-")), false, false)
	default:
		k := scriptKey(what)
		gin.onKey(k, true, false)
		return []Key{k}
	}
	return nil
}
