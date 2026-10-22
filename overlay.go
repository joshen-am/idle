package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Idle sobre la pantalla: una ventana transparente del tamaño del monitor, siempre encima, sin
// ícono, que deja pasar los clics salvo cuando el mouse está sobre la mascota o su menú.
//   - La mascota (pet.go) vive en una esquina. Clic: menú. Arrastrar: moverla o tirarla al
//     basurero para cerrar Idle.
//   - ↑+↓ dos veces (keys.go): modo juego (las teclas son del juego) / modo trabajo.
//   - Para gastar poco: quieto se actualiza 10 veces por segundo; al jugar o arrastrar, 60.

type Scene int

const (
	SceneNone Scene = iota // solo la mascota
	SceneMenu
	SceneProfile
	SceneSearch
	SceneMatch
	SceneStore
	SceneBoard
	SceneDiary
	SceneCPUPick
)

var menuItems = []string{"online", "cpu", "tienda", "ranking", "diario", "perfil", "anim", "sonido", "salir"}

var profLabels = []string{"CUERPO", "COLOR", "OJOS", "GORRO", "NOMBRE", "SALA"}

type btnArea struct {
	x, y, w, h float64
	id         string
}

func (b btnArea) has(x, y float64) bool { return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h }

type Overlay struct {
	cfg    Config
	snd    *Sound
	upd    *Updater
	pet    *Pet
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

	mx, my    float64 // mouse
	down      bool    // botón izquierdo (en cualquier parte de la pantalla)
	prevDown  bool
	pressedIn bool // el clic empezó sobre un botón del panel
	btns      []btnArea
	panelR    btnArea
	passOn    bool // los clics pasan a las apps
	fast      bool
	autoGame  bool // activé el modo juego yo (para escribir o jugar); se apaga al volver
	prevGame  bool
	focusAt   time.Time
	startedAt time.Time
	byeAt     time.Time
	lastSig   string
	sig       string
	sigTick   int
	winX      float64 // dónde está la ventana ahora (puede ser un cuadrito alrededor de la mascota)
	winY      float64
	winW      float64
	winH      float64
	redraws   int
	dbgAt     time.Time
	draws     int
	dbg2      time.Time
	dbgTick   int

	storeTab  int
	storeSel  int
	diary     *Diary
	diaryPage int
	tipToday  bool
	world     *World
	worldDay  string
	recorded  bool   // ya anoté el resultado de esta partida
	coinMsg   string // "+25 ¤" al terminar
	overLine  string // lo que dice el CPU al terminar
	toast     string
	toastAt   time.Time
	nearAt    time.Time // la última vez que el mouse anduvo cerca de la mascota
	owesFocus bool      // hicieron clic en Idle: al irse el mouse, el teclado vuelve a su ventana
	typing    bool      // escribiendo el nombre o la sala
}

var debugDraw = os.Getenv("MJ_DEBUGDRAW") != ""

func NewOverlay() *Overlay {
	o := &Overlay{cfg: loadCfg(), snd: NewSound(), upd: &Updater{}, script: loadScript(), passOn: true,
		startedAt: time.Now()}
	o.snd.Mute = o.cfg.Mute
	saveCfg(o.cfg)
	go o.upd.Check()
	return o
}

// Layout: la ventana mide lo que mide (o.sw y o.sh son el tamaño de la pantalla).
func (o *Overlay) Layout(ow, oh int) (int, int) { return ow, oh }

// initPet pone la mascota en su lugar (o en la esquina de abajo a la derecha).
func (o *Overlay) initPet() {
	x, y := float64(o.sw)-80, float64(o.sh)-bottomMargin()
	if o.cfg.HasPet {
		x, y = o.cfg.PetX*float64(o.sw), o.cfg.PetY*float64(o.sh)
	}
	o.pet = NewPet(clamp(x, 40, float64(o.sw)-40), clamp(y, 70, float64(o.sh)-4))
}

// wantFull: ¿hace falta la ventana del tamaño de la pantalla? Si no, alcanza un cuadrito
// alrededor de la mascota (pintar eso casi no cuesta).
// Se agranda apenas el mouse se acerca (no al hacer clic): cambiar el tamaño de la ventana
// la congela un instante, y así ese instante no se nota.
func (o *Overlay) wantFull() bool {
	return o.scene != SceneNone || o.match != nil || o.pet.drag || o.pet.Dying() || !o.byeAt.IsZero() ||
		time.Since(o.nearAt) < 3*time.Second
}

const smallW, smallH = 260.0, 220.0

// placeWindow agranda o achica la ventana según haga falta.
func (o *Overlay) placeWindow() {
	x, y, w, h := 0.0, 0.0, float64(o.sw), float64(o.sh)
	if runtime.GOOS == "windows" {
		h-- // si ocupa exacto el monitor, Windows la trata como pantalla completa y esconde la barra de tareas
	}
	if !o.wantFull() {
		w, h = smallW, smallH
		x = math.Round(clamp(o.pet.cx-w/2, 0, float64(o.sw)-w))
		y = math.Round(clamp(o.pet.by-h+40, 0, float64(o.sh)-h))
	}
	if x != o.winX || y != o.winY || w != o.winW || h != o.winH {
		if w < o.winW { // al achicar: primero el tamaño (así no tapa nada en el camino)
			ebiten.SetWindowSize(int(w), int(h))
			ebiten.SetWindowPosition(int(x), int(y))
		} else {
			ebiten.SetWindowPosition(int(x), int(y))
			ebiten.SetWindowSize(int(w), int(h))
		}
		o.winX, o.winY, o.winW, o.winH = x, y, w, h
		o.lastSig = "" // hay que pintar de nuevo
	}
}

func bottomMargin() float64 {
	switch runtime.GOOS {
	case "darwin":
		return 90 // sobre el Dock
	case "windows":
		return 54 // sobre la barra de tareas
	}
	return 40
}

func (o *Overlay) savePet() {
	if o.sw > 0 {
		o.cfg.PetX, o.cfg.PetY, o.cfg.HasPet = o.pet.cx/float64(o.sw), o.pet.by/float64(o.sh), true
		saveCfg(o.cfg)
	}
}

func (o *Overlay) trashPos() (float64, float64) { return float64(o.sw) / 2, float64(o.sh) - 120 }

// ---------- acciones ----------

func (o *Overlay) setGame(on bool) {
	gin.SetGame(on)
	o.prevGame = on
}

func (o *Overlay) startCPU(cpu *CPUChar) {
	o.closeMatch()
	o.match = NewCPUMatch(cpu, o.cfg.Av, o.snd)
	o.recorded = false
	o.scene = SceneMatch
	o.setGame(true) // las flechas pasan a ser del juego
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
	if o.scene == SceneMatch || o.scene == SceneSearch {
		o.setGame(false)
	}
	o.scene, o.act = SceneNone, false
	o.recorded = false
}

func (o *Overlay) openMenu() {
	o.scene, o.sel = SceneMenu, 0
}

// isPanel: escenas que son un recuadro al lado de la mascota (un clic afuera las cierra).
func (o *Overlay) isPanel() bool {
	switch o.scene {
	case SceneMenu, SceneProfile, SceneStore, SceneBoard, SceneDiary, SceneCPUPick:
		return true
	}
	return false
}

func (o *Overlay) closePanel() {
	if o.scene == SceneProfile || o.scene == SceneStore {
		if o.cfg.Room == "" {
			o.cfg.Room = "IDLE"
		}
		saveCfg(o.cfg)
	}
	o.stopTyping()
	o.scene = SceneNone
}

func (o *Overlay) openProfile() {
	o.scene, o.sel = SceneProfile, -1
}

// startTyping: clic en NOMBRE o SALA; mientras tanto las teclas son de Idle.
func (o *Overlay) startTyping(row int) {
	o.sel, o.typing = row, true
	if _, game := gin.Peek(); !game {
		o.setGame(true)
		o.autoGame = true
	}
}

func (o *Overlay) stopTyping() {
	o.typing = false
	if o.autoGame {
		o.setGame(false)
		o.autoGame = false
	}
}

func (o *Overlay) profChange(r, d int) {
	o.cfg.nextOwned([]string{"body", "color", "eyes", "hat"}[r], d) // solo lo que ya es tuyo
	saveCfg(o.cfg)
}

func (o *Overlay) menuAction(id string) {
	switch id {
	case "online":
		o.startOnline()
	case "cpu":
		o.scene, o.sel = SceneCPUPick, 0
	case "tienda":
		o.scene, o.storeSel = SceneStore, int(*slot(&o.cfg.Av, storeCats[o.storeTab].key))
	case "ranking":
		o.scene = SceneBoard
	case "diario":
		o.openDiary()
	case "perfil":
		o.openProfile()
	case "anim":
		o.cfg.NoAnim = !o.cfg.NoAnim
		saveCfg(o.cfg)
	case "sonido":
		o.snd.Mute = !o.snd.Mute
		o.cfg.Mute = o.snd.Mute
		saveCfg(o.cfg)
	case "salir":
		o.byeAt = time.Now()
		o.scene = SceneNone
	}
}

// handleKey: una tecla apretada en modo juego.
func (o *Overlay) handleKey(k Key) {
	if k == 'M' && o.scene != SceneProfile {
		o.menuAction("sonido")
		return
	}
	switch o.scene {
	case SceneNone:
		o.openMenu() // cualquier tecla en modo juego abre el menú de la mascota
	case SceneMenu:
		switch k {
		case KUp:
			o.sel = (o.sel + len(menuItems) - 1) % len(menuItems)
		case KDown:
			o.sel = (o.sel + 1) % len(menuItems)
		case KEnter, KSpace, KRight:
			o.menuAction(menuItems[o.sel])
		case KEsc:
			o.scene = SceneNone
			o.setGame(false) // Esc en el menú vuelve al trabajo
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
			o.closePanel()
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
	case SceneStore, SceneBoard, SceneDiary, SceneCPUPick:
		o.sceneKey(k)
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
	if !o.typing {
		return nil
	}
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
	if o.pet == nil {
		o.initPet()
	}
	if gin.Status() != HookOK {
		pollWindowKeys()
	}
	o.runScript()
	cx, cy := ebiten.CursorPosition()
	o.mx, o.my = float64(cx)+o.winX, float64(cy)+o.winY // en coordenadas de la pantalla
	o.prevDown, o.down = o.down, globalMouseDown()
	if botMouse != nil {
		o.mx, o.my, o.down = botMouse(o)
	}
	pressed, released := o.down && !o.prevDown, !o.down && o.prevDown

	// teclado: el gesto ↑+↓ dos veces
	keys, game := gin.Frame()
	if game != o.prevGame {
		o.prevGame = game
		switch {
		case game && o.scene == SceneNone:
			o.openMenu() // entrar al modo juego sin partida: aparece el menú
		case !game && o.isPanel() && o.scene != SceneDiary:
			o.closePanel()
		}
	}
	for _, k := range keys {
		o.handleKey(k)
	}

	// mouse en los paneles: elegir con un clic
	inPanel := o.scene != SceneNone && o.scene != SceneMatch && o.panelR.has(o.mx, o.my) ||
		o.scene == SceneMatch && o.overButton()
	if pressed {
		o.pressedIn = inPanel
	}
	for i, b := range o.btns {
		if b.has(o.mx, o.my) && o.scene == SceneMenu {
			o.sel = i
		}
	}
	if released && o.pressedIn {
		for _, b := range o.btns {
			if b.has(o.mx, o.my) {
				o.clickButton(b.id)
				break
			}
		}
		o.pressedIn = false
		o.afterClick()
	} else if pressed && !inPanel && !o.pet.Hit(o.mx, o.my) && o.isPanel() {
		o.closePanel() // clic afuera: se cierra el menú
	}

	// la mascota (escondida mientras se juega)
	if o.scene != SceneMatch && o.byeAt.IsZero() {
		tx, ty := o.trashPos()
		click, rel, trashed := o.pet.Update(o.mx, o.my, o.down && !o.pressedIn, !o.cfg.NoAnim, float64(o.sw), float64(o.sh), tx, ty)
		if click {
			if o.isPanel() {
				o.closePanel()
			} else if o.scene == SceneNone {
				o.openMenu()
			}
		}
		if rel && !trashed && !click {
			o.savePet()
		}
		if trashed {
			o.byeAt = time.Now()
			logf("mascota: al basurero, Idle se cierra")
		}
		if rel {
			o.afterClick()
		}
	}
	if !o.byeAt.IsZero() && time.Since(o.byeAt) > 1100*time.Millisecond {
		o.quit = true
	}

	// partida
	if o.scene == SceneSearch && o.nw != nil && o.nw.Connected() {
		mode := ModeGuest
		if o.nw.IsHost() {
			mode = ModeHost
		}
		o.match, o.nw = NewMatch(mode, o.nw, o.cfg.Av, o.snd), nil
		o.match.prof = o.cfg.MyProfile()
		o.recorded = false
		o.scene = SceneMatch
		o.setGame(true)
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
		if o.match != nil {
			o.afterMatchFrame()
		}
	}
	o.checkDiary()

	o.updateWindow()
	o.placeWindow()

	if st, _ := o.upd.Status(); st == "listo" && o.match == nil && o.nw == nil && o.scene == SceneNone && !o.pet.Busy() {
		if o.updAt == 0 {
			o.updAt = o.tick + 30
		} else if o.tick >= o.updAt && o.upd.Apply() {
			o.quit = true
		}
	}
	if o.quit {
		o.closeMatch()
		if o.pet != nil && o.byeAt.IsZero() {
			o.savePet()
		}
		saveCfg(o.cfg)
		return ebiten.Termination
	}
	return nil
}

func (o *Overlay) overButton() bool {
	for _, b := range o.btns {
		if b.has(o.mx, o.my) {
			return true
		}
	}
	return false
}

func (o *Overlay) clickButton(id string) {
	switch {
	case o.scene == SceneMenu:
		o.menuAction(id)
	case id == "listo" || id == "cerrar":
		o.closePanel()
	case id == "cancelar":
		o.closeMatch()
	case id == "vscpu":
		o.closeMatch()
		o.scene, o.sel = SceneCPUPick, 0
	case len(id) > 3 && (id[:3] == "st:" || id[:3] == "dy:" || id[:3] == "cp:"):
		o.sceneClick(id)
	case len(id) == 6 && id[:3] == "pf:": // pf:R+ / pf:R-
		r := int(id[3] - '0')
		o.sel = r
		if id[5] == '+' {
			o.profChange(r, 1)
		} else {
			o.profChange(r, -1)
		}
	case len(id) == 4 && id[:3] == "pf:":
		if r := int(id[3] - '0'); r >= 4 {
			o.startTyping(r)
		} else {
			o.stopTyping()
			o.sel = r
		}
	case id == "revancha":
		o.act = true
	case id == "salir":
		o.closeMatch()
	}
}

// afterClick: terminó un clic sobre Idle; el teclado vuelve a tu ventana (salvo que haga falta
// para escribir y no haya teclado de sistema).
func (o *Overlay) afterClick() {
	if o.typing && gin.Status() != HookOK {
		return
	}
	o.owesFocus = true // se devuelve cuando el mouse se va (devolverlo en cada clic hace saltar la pantalla)
}

// updateWindow decide si los clics pasan a las apps y cuán seguido se actualiza Idle.
func (o *Overlay) updateWindow() {
	near := math.Hypot(o.mx-o.pet.cx, o.my-(o.pet.by-6*o.pet.sc)) < 160
	if math.Hypot(o.mx-o.pet.cx, o.my-(o.pet.by-6*o.pet.sc)) < 260 {
		o.nearAt = time.Now()
	}
	interactive := o.pressedIn || (o.scene == SceneMatch && o.overButton()) ||
		o.scene != SceneMatch && (o.pet.Hit(o.mx, o.my) || o.pet.pressed || o.pet.drag ||
			(o.scene != SceneNone && o.panelR.has(o.mx, o.my)))
	if pass := !interactive; pass != o.passOn {
		o.passOn = pass
		ebiten.SetWindowMousePassthrough(pass)
		if pass && o.owesFocus && !(o.typing && gin.Status() != HookOK) {
			restoreFocus()
			o.owesFocus = false
		}
	}
	if o.passOn && time.Since(o.focusAt) > 500*time.Millisecond {
		trackFocus() // recordar tu ventana activa, para devolverle el teclado
		o.focusAt = time.Now()
	}
	moving := o.pet.act == "saltito" || o.pet.act == "estirar" || o.pet.act == "bostezo"
	fast := o.scene != SceneNone || o.pet.pressed || o.pet.drag || o.pet.Dying() || near || moving ||
		!o.byeAt.IsZero() || time.Since(o.pet.landAt) < time.Second || len(o.script) > 0 || botMouse != nil
	if fast != o.fast {
		o.fast = fast
		if fast {
			ebiten.SetTPS(60)
		} else {
			ebiten.SetTPS(10) // quieto: 10 veces por segundo alcanza
		}
	}
}

// signature resume todo lo que se ve cuando la mascota está quieta. Si no cambió desde el
// último cuadro, no se vuelve a pintar (así casi no gasta: la pantalla queda como estaba).
func (o *Overlay) signature() string {
	p := o.pet
	pose, dy := p.pose(o.mx, o.my, !o.cfg.NoAnim)
	t := animTick()
	breath, blink := (t/30)%2, t%200 < 7
	if o.cfg.NoAnim {
		breath, blink = 0, false
	}
	st, _ := o.upd.Status()
	sig := fmt.Sprintf("%v|%d|%v|%.0f|%.0f|%.0f|%v|%d|%v|%s", pose, breath, blink, p.cx, p.by, dy, p.flip,
		gin.Status(), time.Since(o.startedAt) < 7*time.Second, st)
	now := time.Now()
	for _, q := range p.parts { // las partículas se mueven de a saltos de 6 px para repintar menos
		age := now.Sub(q.born).Seconds()
		sig += fmt.Sprintf("|%s%.0f,%.0f,%.1f", q.s, (q.x+q.vx*age)/6, (q.y+q.vy*age)/6, math.Round(clamp(1-age/q.life, 0, 1)*4)/4)
	}
	return sig
}

// runScript ejecuta el guion de teclas de prueba (MJ_SCRIPT).
func (o *Overlay) runScript() {
	for _, k := range o.relKey {
		gin.onKey(k, false, false)
	}
	o.relKey = nil
	t := time.Since(o.startedAt).Seconds()
	for len(o.script) > 0 && o.script[0].at <= t {
		o.relKey = append(o.relKey, runScriptStep(o.script[0].what)...)
		o.script = o.script[1:]
	}
}

// botMouse: pruebas automáticas del mouse (ver mascota_test.go); nil en la app real.
var botMouse func(o *Overlay) (float64, float64, bool)

// ---------- dibujo ----------

var shadowCol = color.NRGBA{0, 0, 0, 110}

// srect: figura con sombra, para que se lea sobre cualquier fondo (claro u oscuro).
func srect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	rect(dst, x+2, y+2, w, h, shadowCol)
	rect(dst, x, y, w, h, c)
}

func stxt(dst *ebiten.Image, s string, x, y, size float64, c color.NRGBA, align int) {
	txt(dst, s, x+2, y+2, size, color.NRGBA{0, 0, 0, uint8(float64(c.A) * .6)}, align)
	txt(dst, s, x, y, size, c, align)
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

// bigButton: botón que se lee sobre cualquier fondo (en la cancha).
func (o *Overlay) bigButton(s *ebiten.Image, id, label string, x, y, w float64, bg color.NRGBA) {
	b := o.button(id, x, y, w, 30)
	fg := colInk
	if bg == colAcc {
		fg = colBg
	}
	if b.has(o.mx, o.my) {
		bg = color.NRGBA{bg.R + (255-bg.R)/5, bg.G + (255-bg.G)/5, bg.B + (255-bg.B)/5, bg.A}
	}
	srect(s, b.x, b.y, b.w, b.h, bg)
	frame(s, b.x, b.y, b.w, b.h, colLine)
	txt(s, label, b.x+b.w/2, b.y+8, 16, fg, 0)
}

// panel: recuadro oscuro al lado de la mascota. Devuelve su esquina superior izquierda.
func (o *Overlay) panel(dst *ebiten.Image, w, h float64) (float64, float64) {
	p := o.pet
	x := p.cx + 7*p.sc + 12
	if p.cx > float64(o.sw)/2 { // si la mascota está a la derecha, el menú sale a su izquierda
		x = p.cx - 7*p.sc - 12 - w
	}
	y := clamp(p.by-h, 16, float64(o.sh)-h-16)
	x = clamp(x, 16, float64(o.sw)-w-16)
	o.panelR = btnArea{x, y, w, h, ""}
	rect(dst, x+3, y+3, w, h, shadowCol)
	rect(dst, x, y, w, h, color.NRGBA{20, 20, 19, 230})
	frame(dst, x, y, w, h, colLine)
	return x, y
}

func (o *Overlay) button(id string, x, y, w, h float64) btnArea {
	b := btnArea{x, y, w, h, id}
	o.btns = append(o.btns, b)
	return b
}

func (o *Overlay) Draw(s *ebiten.Image) {
	if o.pet == nil {
		return
	}
	if debugDraw {
		o.draws++
		if time.Since(o.dbg2) > 5*time.Second {
			logf("motor: %.1f Draw/s %.1f Update/s rapido=%v anim=%v", float64(o.draws)/time.Since(o.dbg2).Seconds(), float64(o.tick-o.dbgTick)/time.Since(o.dbg2).Seconds(), o.fast, !o.cfg.NoAnim)
			o.draws, o.dbg2, o.dbgTick = 0, time.Now(), o.tick
		}
	}
	drawOX, drawOY = -o.winX, -o.winY
	if !o.fast {
		if o.sigTick != o.tick { // una vez por actualización (10 por segundo), no en cada cuadro
			o.sig, o.sigTick = o.signature(), o.tick
		}
		if sig := o.sig; sig == o.lastSig {
			return // no cambió nada: la pantalla queda como estaba, sin gastar
		} else {
			o.lastSig = sig
		}
	} else {
		o.lastSig = ""
	}
	s.Clear()
	if debugDraw {
		o.redraws++
		if time.Since(o.dbgAt) > 5*time.Second {
			logf("dibujo: %.1f repintadas/s rapido=%v act=%q dormido=%v particulas=%d", float64(o.redraws)/time.Since(o.dbgAt).Seconds(), o.fast, o.pet.act, o.pet.asleep, len(o.pet.parts))
			o.redraws, o.dbgAt = 0, time.Now()
		}
	}
	o.btns = o.btns[:0]
	if o.scene == SceneNone || o.scene == SceneMatch {
		o.panelR = btnArea{}
	}
	_, game := gin.Peek()
	if o.scene == SceneMatch {
		if game {
			o.drawMatch(s)
		}
		shot(s, o.tick)
		return
	}
	tx, ty := o.trashPos()
	if o.pet.drag || o.pet.Dying() {
		drawTrash(s, tx, ty, o.pet.overTrash, 0)
		if o.pet.drag {
			msg, c := "SUELTALO AQUI PARA CERRAR IDLE", colInk
			if o.pet.overTrash {
				msg, c = "¡SUELTA PARA CERRAR IDLE!", colErr
			}
			ptxt(s, msg, tx, ty+52, 8, c, 0)
		}
	}
	if o.byeAt.IsZero() || o.pet.Dying() {
		o.pet.Draw(s, o.cfg.Av, o.mx, o.my, !o.cfg.NoAnim, tx, ty)
	}
	if o.scene == SceneNone && !o.pet.drag { // globitos sobre la mascota
		var lines []string
		var cols []color.NRGBA
		if time.Since(o.startedAt) < 7*time.Second {
			lines, cols = append(lines, "¡HOLA! HAZME CLIC"), append(cols, colInk)
		}
		if gin.Status() == HookPermission && time.Since(o.startedAt) < 20*time.Second {
			lines, cols = append(lines, "PARA JUGAR CON FLECHAS:", "DAME PERMISO DE"+" ACCESIBILIDAD"), append(cols, colAcc, colAcc)
		}
		if gin.Status() == HookFailed {
			lines, cols = append(lines, "NO PUDE LEER EL TECLADO"), append(cols, colErr)
		}
		if st, v := o.upd.Status(); st == "listo" {
			lines, cols = append(lines, "ACTUALIZANDO A V"+v), append(cols, colOk)
		}
		for i := range lines {
			ptxt(s, lines[i], o.pet.cx, o.pet.by-12*o.pet.sc-24-float64(len(lines)-1-i)*20, 8, cols[i], 0)
		}
	}
	switch o.scene {
	case SceneMenu:
		o.drawMenu(s)
	case SceneProfile:
		o.drawProfile(s)
	case SceneSearch:
		o.drawSearch(s)
	case SceneStore:
		o.drawStore(s)
	case SceneBoard:
		o.drawBoard(s)
	case SceneDiary:
		o.drawDiary(s)
	case SceneCPUPick:
		o.drawCPUPick(s)
	}
	shot(s, o.tick)
}

func (o *Overlay) drawMenu(s *ebiten.Image) {
	labels := map[string]string{"online": "PONG ONLINE", "cpu": "PONG VS CPU", "tienda": "TIENDA", "ranking": "RANKING",
		"diario": "DIARIO", "perfil": "PERSONAJE",
		"anim": "ANIMACIONES: SI", "sonido": "SONIDO: NO", "salir": "CERRAR IDLE"}
	if o.cfg.NoAnim {
		labels["anim"] = "ANIMACIONES: NO"
	}
	if !o.snd.Mute {
		labels["sonido"] = "SONIDO: SI"
	}
	w, h := 300.0, 30+float64(len(menuItems))*28+34
	x, y := o.panel(s, w, h)
	txt(s, o.cfg.Av.DisplayName(), x+16, y+12, 8, o.cfg.Av.Col(), -1)
	txt(s, fmt.Sprintf("★%d  ¤%d", total(o.cfg.Wins()), o.cfg.Coins), x+w-16, y+12, 8, colAcc, 1)
	for i, id := range menuItems {
		b := o.button(id, x+6, y+30+float64(i)*28, w-12, 26)
		c := colDim
		if i == o.sel {
			rect(s, b.x, b.y, b.w, b.h, colCard)
			txt(s, "▶", b.x+8, b.y+6, 16, colAcc, -1)
			c = colInk
		}
		txt(s, labels[id], b.x+30, b.y+6, 16, c, -1)
	}
	txt(s, "ARRASTRAME AL BASURERO PARA CERRAR", x+w/2, y+h-22, 8, colLine, 0)
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
	w, h := 420.0, 250.0
	x, y := o.panel(s, w, h)
	txt(s, "PERSONAJE", x+16, y+12, 8, colDim, -1)
	for r, l := range profLabels {
		ry := y + 34 + float64(r)*28
		c := colDim
		if r == o.sel {
			rect(s, x+6, ry-6, w-12, 26, colCard)
			c = colInk
		}
		o.button("pf:"+string(rune('0'+r)), x+6, ry-6, w-12, 26)
		txt(s, l, x+18, ry+4, 8, c, -1)
		val := o.profValue(r)
		if r >= 4 {
			if r == o.sel && o.typing && (o.tick/30)%2 == 0 {
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
		// botones ◀ ▶ (se agregan después para que tengan prioridad sobre la fila)
		o.btns = append([]btnArea{{x + 160, ry - 6, 30, 26, "pf:" + string(rune('0'+r)) + "-"},
			{x + w - 40, ry - 6, 30, 26, "pf:" + string(rune('0'+r)) + "+"}}, o.btns...)
		txt(s, "◀", x+168, ry, 16, colAcc, -1)
		txt(s, val, x+(160+w-10)/2, ry, 16, c, 0)
		txt(s, "▶", x+w-32, ry, 16, colAcc, -1)
	}
	b := o.button("listo", x+w-110, y+h-36, 100, 28)
	rect(s, b.x, b.y, b.w, b.h, colAcc)
	txt(s, "LISTO", b.x+b.w/2, b.y+7, 16, colBg, 0)
	txt(s, "CLIC EN NOMBRE O SALA", x+16, y+h-34, 8, colLine, -1)
	txt(s, "PARA ESCRIBIR", x+16, y+h-20, 8, colLine, -1)
}

func (o *Overlay) drawSearch(s *ebiten.Image) {
	w, h := 330.0, 150.0
	x, y := o.panel(s, w, h)
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
	txt(s, lines[0], x+16, y+18, 16, colInk, -1)
	txt(s, lines[1], x+16, y+42, 16, colInk, -1)
	txt(s, "SALA "+o.cfg.Room+" · EN SU PC: PONG ONLINE", x+16, y+76, 8, colDim, -1)
	b := o.button("cancelar", x+w-130, y+h-40, 120, 28)
	rect(s, b.x, b.y, b.w, b.h, colCard)
	frame(s, b.x, b.y, b.w, b.h, colLine)
	txt(s, "CANCELAR", b.x+b.w/2, b.y+7, 16, colInk, 0)
	b = o.button("vscpu", x+10, y+h-40, 120, 28) // si no llega nadie, juega un CPU
	rect(s, b.x, b.y, b.w, b.h, colCard)
	frame(s, b.x, b.y, b.w, b.h, colLine)
	txt(s, "VS CPU", b.x+b.w/2, b.y+7, 16, colInk, 0)
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
	if p.Ph != PhOver && p.Ph != PhDC {
		o.bigButton(s, "salir", "SALIR", sw-110, 10, 96, colCard)
	}
	switch {
	case p.Ph == PhCount:
		ptxt(s, strconv.Itoa(int(math.Max(1, math.Ceil(p.Tm)))), sw/2, sh/2-16, 32, colInk, 0)
		ptxt(s, "↑↓ MUEVES · ↑ Y ↓ JUNTAS ×2 = TRABAJO", sw/2, sh-60, 8, colDim, 0)
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
		o.bigButton(s, "revancha", "REVANCHA", sw/2-170, sh/2+70, 160, colAcc)
		o.bigButton(s, "salir", "SALIR", sw/2+10, sh/2+70, 160, colCard)
		if o.coinMsg != "" {
			ptxt(s, o.coinMsg, sw/2, sh/2-60, 16, colAcc, 0)
		}
		if o.overLine != "" {
			ptxt(s, o.overLine, sw/2, sh/2+40, 8, colInk, 0)
		}
	case p.Ph == PhDC:
		ptxt(s, "SE CORTO LA CONEXION", sw/2, sh/2-24, 16, colErr, 0)
		o.bigButton(s, "salir", "VOLVER", sw/2-80, sh/2+20, 160, colCard)
	case p.Paused:
		ptxt(s, "UN MOMENTO"+dots(o.tick), sw/2, 52, 8, colDim, 0)
	}
}

// runOverlay abre la ventana transparente sobre toda la pantalla, sin ícono.
func runOverlay() error {
	if !acquireInstance() {
		return nil // ya hay una mascota abierta
	}
	startHook()
	mw, mh := 1280, 800
	if m := ebiten.Monitor(); m != nil {
		mw, mh = m.Size()
	}
	if runtime.GOOS == "windows" {
		mh-- // si ocupa exacto el monitor, Windows la trata como pantalla completa y esconde la barra de tareas
	}
	o := NewOverlay()
	o.sw, o.sh = mw, mh+map[bool]int{true: 1, false: 0}[runtime.GOOS == "windows"]
	o.initPet()
	switch os.Getenv("MJ_DEMO") { // pruebas: arranca directo en una partida
	case "cpu":
		o.startCPU(&cpus[0])
	case "tienda", "ranking", "diario":
		o.menuAction(os.Getenv("MJ_DEMO"))
		fmt.Sscanf(os.Getenv("MJ_PAGE"), "%d", &o.diaryPage)
	case "rival":
		o.menuAction("cpu")

	case "online":
		o.setGame(true)
		o.startOnline()
	}
	ebiten.SetWindowTitle("Idle")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(true)
	ebiten.SetRunnableOnUnfocused(true)
	o.placeWindow()
	ebiten.SetWindowIcon([]image.Image{appIcon(16), appIcon(32), appIcon(64)})
	ebiten.SetScreenClearedEveryFrame(false) // se repinta solo cuando algo cambia (ver Draw)
	ebiten.SetTPS(10)
	return ebiten.RunGameWithOptions(o, &ebiten.RunGameOptions{ScreenTransparent: true, InitUnfocused: true, SkipTaskbar: true})
}
