package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"runtime"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Idle sobre la pantalla: una ventana transparente del tamaño del monitor, siempre encima y que
// deja pasar los clics a lo que haya debajo. Se maneja solo con el teclado (keys.go):
//   - Modo trabajo: no se ve nada y las teclas van a tus apps.
//   - Modo juego (↑+↓ dos veces): aparece un menú chico en la esquina o la partida.

type Scene int

const (
	SceneMenu Scene = iota
	SceneProfile
	SceneSearch
	SceneMatch
)

var menuItems = []string{"online", "cpu", "perfil", "sonido", "salir"}

var profLabels = []string{"CUERPO", "COLOR", "OJOS", "GORRO", "NOMBRE", "SALA"}

type Overlay struct {
	cfg    Config
	snd    *Sound
	upd    *Updater
	scene  Scene
	sel    int
	tick   int
	sw, sh int
	quit   bool
	act    bool // Enter/Espacio pendiente para la partida
	match  *Match
	nw     *Net // red mientras se busca al compañero
	script []scriptStep
	relKey []Key
	updAt  int
}

func NewOverlay() *Overlay {
	o := &Overlay{cfg: loadCfg(), snd: NewSound(), upd: &Updater{}, script: loadScript()}
	o.snd.Mute = o.cfg.Mute
	saveCfg(o.cfg)
	go o.upd.Check()
	return o
}

func (o *Overlay) Layout(ow, oh int) (int, int) {
	o.sw, o.sh = ow, oh
	return ow, oh
}

// ---------- acciones ----------

func (o *Overlay) startCPU() {
	o.closeMatch()
	o.match = NewMatch(ModeCPU, nil, o.cfg.Av, o.snd)
	o.scene = SceneMatch
}

func (o *Overlay) startOnline() {
	o.closeMatch()
	o.nw = AutoRoom(o.cfg.Room)
	o.scene = SceneSearch
}

func (o *Overlay) closeMatch() {
	if o.match != nil {
		o.match.Close()
		o.match = nil
	}
	if o.nw != nil {
		o.nw.Close()
		o.nw = nil
	}
	o.scene, o.act = SceneMenu, false
}

func (o *Overlay) profChange(r, d int) {
	av := &o.cfg.Av
	switch r {
	case 0:
		av.Body = wrap(av.Body, d, len(bodies))
	case 1:
		av.Color = wrap(av.Color, d, len(avColors))
	case 2:
		av.Eyes = wrap(av.Eyes, d, len(eyeNames))
	case 3:
		av.Hat = wrap(av.Hat, d, len(hats))
	}
	saveCfg(o.cfg)
}

// handleKey: una tecla apretada en modo juego.
func (o *Overlay) handleKey(k Key) {
	if k == 'M' && o.scene != SceneProfile {
		o.snd.Mute = !o.snd.Mute
		o.cfg.Mute = o.snd.Mute
		saveCfg(o.cfg)
		return
	}
	switch o.scene {
	case SceneMenu:
		switch k {
		case KUp:
			o.sel = (o.sel + len(menuItems) - 1) % len(menuItems)
		case KDown:
			o.sel = (o.sel + 1) % len(menuItems)
		case KEnter, KSpace, KRight:
			switch menuItems[o.sel] {
			case "online":
				o.startOnline()
			case "cpu":
				o.startCPU()
			case "perfil":
				o.scene, o.sel = SceneProfile, 0
			case "sonido":
				o.handleKey('M')
			case "salir":
				o.quit = true
			}
		case KEsc:
			gin.SetGame(false) // atajo: Esc en el menú vuelve al trabajo
		}
	case SceneProfile:
		switch k {
		case KUp:
			o.sel = (o.sel + len(profLabels) - 1) % len(profLabels)
		case KDown:
			o.sel = (o.sel + 1) % len(profLabels)
		case KLeft, KRight:
			if o.sel < 4 {
				o.profChange(o.sel, map[Key]int{KLeft: -1, KRight: 1}[k])
			}
		case KEnter, KEsc:
			if o.cfg.Room == "" {
				o.cfg.Room = "IDLE"
			}
			saveCfg(o.cfg)
			o.scene, o.sel = SceneMenu, 2
		case KBack:
			if f := o.textField(); f != nil && *f != "" {
				*f = (*f)[:len(*f)-1]
				saveCfg(o.cfg)
			}
		default:
			if f := o.textField(); f != nil && (k >= 'A' && k <= 'Z' || k >= '0' && k <= '9') {
				*f = cleanName(*f + string(rune(k)))
				saveCfg(o.cfg)
			}
		}
	case SceneSearch:
		if k == KEsc {
			o.closeMatch()
		}
	case SceneMatch:
		switch k {
		case KEnter, KSpace:
			o.act = true
		case KEsc:
			o.closeMatch()
		}
	}
}

func (o *Overlay) textField() *string {
	switch o.sel {
	case 4:
		return &o.cfg.Av.Name
	case 5:
		return &o.cfg.Room
	}
	return nil
}

// ---------- Update ----------

func (o *Overlay) Update() error {
	o.tick++
	if gin.Status() == HookOff {
		pollWindowKeys()
	}
	o.runScript()
	keys, game := gin.Frame()
	for _, k := range keys {
		o.handleKey(k)
	}

	if o.scene == SceneSearch && o.nw != nil && o.nw.Connected() {
		mode := ModeGuest
		if o.nw.IsHost() {
			mode = ModeHost
		}
		o.match, o.nw = NewMatch(mode, o.nw, o.cfg.Av, o.snd), nil
		o.scene = SceneMatch
	}
	if o.match != nil {
		var in Input
		if game {
			if gin.Down(KUp) {
				in.Dir--
			}
			if gin.Down(KDown) {
				in.Dir++
			}
		}
		if botMode { // pruebas: sigue la bola sola
			_, by := o.match.BallPos()
			in = Input{Target: by + BS/2, HasTarget: true}
		}
		if o.match.Update(in, o.act, !game) {
			o.closeMatch()
		}
		o.act = false
	}

	if st, _ := o.upd.Status(); st == "listo" && o.match == nil && o.nw == nil {
		if o.updAt == 0 {
			o.updAt = o.tick + 60
		} else if o.tick >= o.updAt && o.upd.Apply() {
			o.quit = true
		}
	}
	if o.quit {
		o.closeMatch()
		saveCfg(o.cfg)
		return ebiten.Termination
	}
	return nil
}

// runScript ejecuta el guion de teclas de prueba (MJ_SCRIPT).
func (o *Overlay) runScript() {
	for _, k := range o.relKey {
		gin.onKey(k, false, false)
	}
	o.relKey = nil
	t := float64(o.tick) / 60
	for len(o.script) > 0 && o.script[0].at <= t {
		o.relKey = append(o.relKey, runScriptStep(o.script[0].what)...)
		o.script = o.script[1:]
	}
}

// ---------- dibujo ----------

var shadowCol = color.NRGBA{0, 0, 0, 110}

// srect: figura con sombra, para que se lea sobre cualquier fondo (claro u oscuro).
func srect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	rect(dst, x+2, y+2, w, h, shadowCol)
	rect(dst, x, y, w, h, c)
}

func textW(s string, size float64) float64 {
	return text.Advance(s, &text.GoTextFace{Source: fontSrc, Size: size})
}

// ptxt: texto sobre una pastilla oscura semitransparente (se lee sobre ventanas claras).
func ptxt(dst *ebiten.Image, s string, x, y, size float64, c color.Color, align int) {
	w := textW(s, size)
	left := x - w/2
	switch align {
	case -1:
		left = x
	case 1:
		left = x - w
	}
	pad := size / 2
	rect(dst, left-pad, y-pad*.8, w+2*pad, size+pad*1.6, color.NRGBA{20, 20, 19, 175})
	txt(dst, s, x, y, size, c, align)
}

var ballCol = hex(0xf2c94c) // amarillo con borde oscuro: se ve sobre fondos claros y oscuros

func ball(dst *ebiten.Image, x, y, w float64, c color.Color) {
	b := math.Max(2, w/6)
	rect(dst, x+2, y+2, w, w, shadowCol)
	rect(dst, x-b, y-b, w+2*b, w+2*b, color.NRGBA{25, 25, 25, 220})
	rect(dst, x, y, w, w, c)
}

// panel: recuadro oscuro en la esquina inferior derecha. Devuelve su esquina superior izquierda.
func (o *Overlay) panel(dst *ebiten.Image, w, h float64) (float64, float64) {
	x, y := float64(o.sw)-w-24, float64(o.sh)-h-24
	rect(dst, x+3, y+3, w, h, shadowCol)
	rect(dst, x, y, w, h, color.NRGBA{20, 20, 19, 225})
	frame(dst, x, y, w, h, colLine)
	return x, y
}

func (o *Overlay) Draw(s *ebiten.Image) {
	_, game := gin.Peek()
	switch st := gin.Status(); {
	case st == HookPermission:
		ptxt(s, "IDLE NECESITA PERMISO DE ACCESIBILIDAD", float64(o.sw)-28, float64(o.sh)-54, 8, colAcc, 1)
		ptxt(s, "AJUSTES > PRIVACIDAD Y SEGURIDAD > ACCESIBILIDAD > IDLE", float64(o.sw)-28, float64(o.sh)-34, 8, colInk, 1)
	case st == HookFailed:
		ptxt(s, "IDLE NO PUDO LEER EL TECLADO", float64(o.sw)-28, float64(o.sh)-34, 8, colErr, 1)
	case !game && o.tick < 60*8:
		ptxt(s, "IDLE · ↑ Y ↓ JUNTAS, DOS VECES = JUGAR", float64(o.sw)-28, float64(o.sh)-34, 8, colInk, 1)
	}
	if st, v := o.upd.Status(); st == "listo" {
		ptxt(s, "IDLE: ACTUALIZANDO A V"+v, float64(o.sw)-28, 24, 8, colOk, 1)
	}
	if game {
		switch o.scene {
		case SceneMenu:
			o.drawMenu(s)
		case SceneProfile:
			o.drawProfile(s)
		case SceneSearch:
			o.drawSearch(s)
		case SceneMatch:
			o.drawMatch(s)
		}
	}
	shot(s, o.tick)
}

func (o *Overlay) drawMenu(s *ebiten.Image) {
	x, y := o.panel(s, 340, 214)
	drawAvatar(s, x+18, y+44, 4, o.cfg.Av, o.tick, false)
	txt(s, o.cfg.Av.DisplayName(), x+42, y+86, 8, o.cfg.Av.Col(), 0)
	labels := map[string]string{"online": "PONG ONLINE", "cpu": "PONG VS CPU", "perfil": "PERSONAJE",
		"sonido": "SONIDO: NO", "salir": "CERRAR IDLE"}
	if !o.snd.Mute {
		labels["sonido"] = "SONIDO: SI"
	}
	for i, id := range menuItems {
		iy := y + 16 + float64(i)*28
		c := colDim
		if i == o.sel {
			rect(s, x+86, iy-6, 242, 26, colCard)
			txt(s, "▶", x+92, iy, 16, colAcc, -1)
			c = colInk
		}
		txt(s, labels[id], x+114, iy, 16, c, -1)
	}
	txt(s, "↑↓ ELEGIR · ENTER · M SONIDO", x+170, y+164, 8, colDim, 0)
	txt(s, "↑ Y ↓ JUNTAS ×2 = TRABAJO", x+170, y+184, 8, colDim, 0)
}

func (o *Overlay) profValue(r int) string {
	av := o.cfg.Av.fix()
	switch r {
	case 0:
		return bodies[av.Body].name
	case 1:
		return avColors[av.Color].name
	case 2:
		return eyeNames[av.Eyes]
	case 3:
		return hats[av.Hat].name
	case 4:
		return o.cfg.Av.Name
	}
	return o.cfg.Room
}

func (o *Overlay) drawProfile(s *ebiten.Image) {
	x, y := o.panel(s, 420, 300)
	drawAvatar(s, x+180, y+36, 5, o.cfg.Av, o.tick, false)
	for r, l := range profLabels {
		ry := y + 96 + float64(r)*26
		c := colDim
		if r == o.sel {
			rect(s, x+10, ry-6, 400, 26, colCard)
			c = colInk
		}
		txt(s, l, x+20, ry+4, 8, c, -1)
		val := o.profValue(r)
		if r >= 4 {
			if r == o.sel && (o.tick/30)%2 == 0 {
				val += "_"
			}
			col := colInk
			if r == 4 {
				col = o.cfg.Av.Col()
			}
			txt(s, val, x+130, ry, 16, col, -1)
			continue
		}
		if r == 1 {
			rect(s, x+130, ry, 16, 16, o.cfg.Av.Col())
		}
		txt(s, "◀ "+val+" ▶", x+300, ry, 16, c, 0)
	}
	txt(s, "↑↓ FILA · ←→ CAMBIAR · ESCRIBE NOMBRE/SALA", x+210, y+260, 8, colDim, 0)
	txt(s, "ENTER LISTO", x+210, y+278, 8, colDim, 0)
}

func (o *Overlay) drawSearch(s *ebiten.Image) {
	x, y := o.panel(s, 340, 150)
	drawAvatar(s, x+18, y+40, 4, o.cfg.Av, o.tick, false)
	st := NetConnecting
	if o.nw != nil {
		st = o.nw.Status()
	}
	lines := map[int][2]string{
		NetConnecting: {"CONECTANDO" + dots(o.tick), ""},
		NetWaiting:    {"ESPERANDO A TU", "AMIGO" + dots(o.tick)},
		NetJoining:    {"ENTRANDO" + dots(o.tick), ""},
		NetNoInternet: {"SIN INTERNET", "REINTENTANDO" + dots(o.tick)},
	}[st]
	txt(s, lines[0], x+96, y+28, 16, colInk, -1)
	txt(s, lines[1], x+96, y+52, 16, colInk, -1)
	txt(s, "SALA "+o.cfg.Room, x+96, y+84, 8, colDim, -1)
	txt(s, "EN SU PC: PONG ONLINE · ESC CANCELA", x+170, y+124, 8, colDim, 0)
}

func (o *Overlay) drawMatch(s *ebiten.Image) {
	m := o.match
	p := m.p
	sw, sh := float64(o.sw), float64(o.sh)
	k := math.Min(sw/W, sh/H) // la cancha usa toda la pantalla
	ox, oy := (sw-W*k)/2, (sh-H*k)/2
	X := func(x float64) float64 { return math.Round(ox + x*k) }
	Y := func(y float64) float64 { return math.Round(oy + y*k) }

	av1, av2 := m.Avatars()
	c1, c2 := av1.Col(), av2.Col()
	srect(s, X(PX), Y(p.A), PW*k, PH*k, c1)
	srect(s, X(W-PX-PW), Y(p.B), PW*k, PH*k, c2)
	ax, ay := X(3+m.pulse1), Y(p.A+PH/2-4)
	bx, by := X(W-15-m.pulse2), Y(p.B+PH/2-4)
	drawAvatarTint(s, ax+2, ay+2, k, av1, o.tick, false, shadowCol)
	drawAvatar(s, ax, ay, k, av1, o.tick, false)
	drawAvatarTint(s, bx+2, by+2, k, av2, o.tick+40, true, shadowCol)
	drawAvatar(s, bx, by, k, av2, o.tick+40, true)

	if p.Ph == PhPlay {
		for i, t := range m.trail {
			al := float64(i+1) / float64(len(m.trail)) * .35
			rect(s, X(t[0]+1), Y(t[1]+1), (BS-2)*k, (BS-2)*k, fade(shade(ballCol, .8), al))
		}
		bxp, byp := m.BallPos()
		ball(s, X(bxp), Y(byp), BS*k, ballCol)
	}

	ptxt(s, fmt.Sprintf("%s %d · %d %s", av1.DisplayName(), p.S1, p.S2, av2.DisplayName()), sw/2, 16, 16, colInk, 0)
	switch {
	case p.Ph == PhCount:
		ptxt(s, strconv.Itoa(int(math.Max(1, math.Ceil(p.Tm)))), sw/2, sh/2-16, 32, colInk, 0)
		ptxt(s, "↑↓ MUEVES · ESC MENU · ↑ Y ↓ JUNTAS ×2 = TRABAJO", sw/2, sh-60, 8, colDim, 0)
		if m.Online() {
			via, c := "POR SERVIDOR", colDim
			if m.nw.Direct() {
				via, c = "DIRECTO", colOk
			}
			ptxt(s, via, sw/2, sh/2+40, 8, c, 0)
		}
	case p.Ph == PhOver:
		w := av1
		if p.Winner == 2 {
			w = av2
		}
		ptxt(s, "GANA "+w.DisplayName(), sw/2, sh/2-24, 16, w.Col(), 0)
		ptxt(s, "ENTER REVANCHA · ESC MENU", sw/2, sh/2+12, 8, colInk, 0)
	case p.Ph == PhDC:
		ptxt(s, "SE CORTO LA CONEXION", sw/2, sh/2-24, 16, colErr, 0)
		ptxt(s, "ENTER VOLVER", sw/2, sh/2+12, 8, colInk, 0)
	case p.Paused:
		ptxt(s, "UN MOMENTO"+dots(o.tick), sw/2, 52, 8, colDim, 0)
	}
}

// runOverlay abre la ventana transparente sobre toda la pantalla.
func runOverlay() error {
	startHook()
	mw, mh := 1280, 800
	if m := ebiten.Monitor(); m != nil {
		mw, mh = m.Size()
	}
	if runtime.GOOS == "windows" {
		mh-- // si ocupa exacto el monitor, Windows la trata como pantalla completa y esconde la barra de tareas
	}
	o := NewOverlay()
	switch os.Getenv("MJ_DEMO") { // pruebas: arranca directo en una partida
	case "cpu":
		gin.SetGame(true)
		o.startCPU()
	case "online":
		gin.SetGame(true)
		o.startOnline()
	}
	ebiten.SetWindowTitle("Idle")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(true)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowSize(mw, mh)
	ebiten.SetWindowPosition(0, 0)
	ebiten.SetWindowIcon([]image.Image{appIcon(16), appIcon(32), appIcon(64)})
	return ebiten.RunGameWithOptions(o, &ebiten.RunGameOptions{ScreenTransparent: true, InitUnfocused: true})
}
