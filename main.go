package main

import (
	"encoding/json"
	"image"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ---------- configuración guardada (personaje, sonido, tamaño, posición) ----------

type Config struct {
	Av     Avatar
	HasAv  bool
	Mute   bool
	Scale  int // índice en winSizes
	Spot   int // 0 abajo-derecha, 1 abajo-centro, 2 abajo-izquierda
	HasPos bool
	X, Y   int
	LastIP string
}

var winSizes = []int{240, 300, 360, 480}

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
	if c.Scale < 0 || c.Scale >= len(winSizes) {
		c.Scale = 0
	}
	if !c.HasAv {
		c.Av, c.HasAv = randomAvatar(), true
	}
	c.Av = c.Av.fix()
	return c
}

func (a *App) saveCfg() {
	p := cfgPath()
	if p == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(a.cfg, "", "  ")
	os.WriteFile(p, b, 0o644)
}

// ---------- app ----------

type Scene int

const (
	SMenu Scene = iota
	SPong
	SHost
	SJoin
	SGame
	SProfile
)

type Mode int

const (
	ModeLocal Mode = iota
	ModeCPU
	ModeHost
	ModeGuest
)

type btn struct {
	x, y, w, h float64
	id         string
}

func (b btn) has(x, y float64) bool { return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h }

type App struct {
	upd   *Updater
	updAt int
	cfg   Config
	snd   *Sound
	scene Scene
	sel   int
	tick  int
	quit  bool

	// tecla de pánico (Tab): tapa todo con un "monitor del sistema"
	panic   bool
	cpuHist []float64

	// juego
	mode                         Mode
	pong                         *Pong
	lastEv                       [EvCount]uint8
	evReady                      bool
	rematch                      uint8
	lastRematch                  uint8
	lastSeq                      uint16
	targetA                      float64
	hitPending                   bool   // invitado: golpeé y espero que el anfitrión lo confirme
	hitMsg                       HitPkt // el golpe que estoy avisando
	hitAt                        int
	skipHit2                     uint8
	lat                          float64 // demora estimada de ida (invitado)
	corrX, corrY                 float64 // corrección pendiente de la bola (invitado)
	targetB                      float64 // paleta del invitado, suavizada (anfitrión)
	shake, flash, pulse1, pulse2 float64
	trail                        [][2]float64

	// red
	nw        *Net
	joinStart time.Time
	ipInput   string
	status    string
	statusErr bool

	// ratón
	cx, cy         float64
	lastCX, lastCY float64
	ptrActive      bool
	ptrY           float64
	ptrSide        int
	dragging       bool
	dragX, dragY   float64
	placed         bool
}

func NewApp() *App {
	a := &App{cfg: loadCfg(), snd: NewSound(), upd: &Updater{}}
	a.snd.Mute = a.cfg.Mute
	a.cpuHist = make([]float64, 36)
	for i := range a.cpuHist {
		a.cpuHist[i] = 8 + rand.Float64()*14
	}
	a.saveCfg()
	go a.upd.Check()
	return a
}

func (a *App) go_(s Scene) {
	a.scene, a.sel, a.status, a.statusErr = s, 0, "", false
}

func (a *App) back() {
	switch a.scene {
	case SGame, SHost, SJoin:
		a.nw.Close()
		a.nw, a.pong = nil, nil
		a.go_(SPong)
	case SPong, SProfile:
		a.go_(SMenu)
	}
}

func (a *App) startGame(m Mode) {
	a.mode, a.pong = m, NewPong()
	a.lastEv, a.evReady, a.rematch, a.lastRematch = [EvCount]uint8{}, m != ModeGuest, 0, 0
	a.ptrActive, a.trail = false, nil
	a.go_(SGame)
}

func (a *App) hostRoom() {
	a.nw.Close()
	a.go_(SHost)
	a.nw = HostRoom()
}

func (a *App) openJoin() {
	a.nw.Close()
	a.go_(SJoin)
	a.nw = NewSearcher()
}

func (a *App) joinAddr(addr *net.UDPAddr) {
	a.nw.Close()
	a.nw = NewSearcher()
	if err := a.nw.JoinLAN(addr); err != nil {
		a.status, a.statusErr = "ERROR DE RED", true
		return
	}
	a.joinStart = time.Now()
	a.status, a.statusErr = "CONECTANDO", false
}

// joinInput: lo escrito puede ser el código de 4 letras (por internet) o una IP (red local).
func (a *App) joinInput() {
	in := strings.ToUpper(strings.TrimSpace(a.ipInput))
	if strings.Contains(in, ".") {
		ip := net.ParseIP(in).To4()
		if ip == nil {
			a.status, a.statusErr = "IP NO VALIDA", true
			return
		}
		a.cfg.LastIP = in
		a.saveCfg()
		a.joinAddr(&net.UDPAddr{IP: ip, Port: GamePort})
		return
	}
	if len(in) != 4 {
		a.status, a.statusErr = "EL CODIGO TIENE 4", true
		return
	}
	a.nw.Close()
	a.nw = NewSearcher()
	a.nw.JoinCode(in)
	a.joinStart = time.Now()
	a.status, a.statusErr = "CONECTANDO", false
}

// ---------- ventana ----------

func (a *App) bottomMargin() int {
	switch runtime.GOOS {
	case "darwin":
		return 84 // por encima del Dock
	case "windows":
		return 60 // por encima de la barra de tareas
	}
	return 48
}

func (a *App) placeWindow() {
	m := ebiten.Monitor()
	if m == nil {
		return
	}
	mw, mh := m.Size()
	s := winSizes[a.cfg.Scale]
	if a.cfg.HasPos && a.cfg.X >= -s/2 && a.cfg.Y >= 0 && a.cfg.X < mw-s/2 && a.cfg.Y < mh-s/2 {
		ebiten.SetWindowPosition(a.cfg.X, a.cfg.Y)
		return
	}
	xs := []int{mw - s - 12, (mw - s) / 2, 12}
	ebiten.SetWindowPosition(xs[a.cfg.Spot%3], mh-s-a.bottomMargin())
}

func (a *App) cycleSpot() {
	a.cfg.Spot = (a.cfg.Spot + 1) % 3
	a.cfg.HasPos = false
	a.placeWindow()
	a.saveCfg()
}

func (a *App) resize(d int) {
	ns := a.cfg.Scale + d
	if ns < 0 || ns >= len(winSizes) {
		return
	}
	old, nw := winSizes[a.cfg.Scale], winSizes[ns]
	a.cfg.Scale = ns
	ebiten.SetWindowSize(nw, nw)
	if a.cfg.HasPos { // que crezca hacia arriba (y hacia la izquierda si está a la derecha)
		x, y := ebiten.WindowPosition()
		y += old - nw
		if m := ebiten.Monitor(); m != nil {
			if mw, _ := m.Size(); x+old/2 > mw/2 {
				x += old - nw
			}
		}
		a.cfg.X, a.cfg.Y = x, y
	}
	a.placeWindow()
	a.saveCfg()
}

// ---------- botones de cada pantalla ----------

func (a *App) topButtons() []btn {
	bs := []btn{}
	if a.scene != SMenu && !a.panic {
		bs = append(bs, btn{4, 4, 12, 12, "back"})
	}
	return append(bs, btn{196, 4, 12, 12, "mute"}, btn{210, 4, 12, 12, "pos"}, btn{224, 4, 12, 12, "close"})
}

var menuRows = []struct {
	id, label string
	on        bool
}{
	{"pong", "PONG", true},
	{"", "SNAKE", false},
	{"", "GATO", false},
	{"", "HOCKEY", false},
	{"perfil", "PERSONAJE", true},
}

var menuSel = []int{0, 4} // filas que se pueden elegir con el teclado

func menuBtn(i int) btn {
	y := 44 + float64(i)*22
	if i == 4 {
		y += 12
	}
	return btn{84, y, 148, 20, menuRows[i].id}
}

func profBtn(r, d int) btn {
	x := 104.0
	if d > 0 {
		x = 220
	}
	return btn{x, 104 + float64(r)*20 - 2, 12, 12, "pf:" + string(rune('0'+r)) + ":" + map[int]string{-1: "-1", 1: "1"}[d]}
}

var modeItems = []struct{ id, title, sub string }{
	{"local", "MISMO PC", "W/S CONTRA FLECHAS"},
	{"cpu", "VS CPU", "PRACTICA SOLO"},
	{"host", "CREAR SALA", "TE DA UN CODIGO"},
	{"join", "UNIRSE", "ESCRIBES EL CODIGO"},
}

func (a *App) buttons() []btn {
	bs := a.topButtons()
	if a.panic {
		return bs
	}
	switch a.scene {
	case SMenu:
		for i, r := range menuRows {
			if r.on {
				bs = append(bs, menuBtn(i))
			}
		}
	case SPong:
		for i, m := range modeItems {
			bs = append(bs, btn{10, 38 + float64(i)*44, 220, 38, m.id})
		}
	case SProfile:
		for r := 0; r < 4; r++ {
			bs = append(bs, profBtn(r, -1), profBtn(r, 1))
		}
		bs = append(bs, btn{4, 180, W - 8, 18, "pfname"})
	case SJoin:
		if a.nw != nil {
			for i, r := range a.nw.Rooms() {
				if i < 3 {
					bs = append(bs, btn{10, 120 + float64(i)*30, 220, 26, "room:" + r.Addr.String()})
				}
			}
		}
		bs = append(bs, btn{168, 42, 42, 30, "ipok"})
	}
	return bs
}

func wrap(v uint8, d, n int) uint8 { return uint8(((int(v)+d)%n + n) % n) }

func (a *App) profChange(r, d int) {
	av := &a.cfg.Av
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
	a.sel = r
	a.saveCfg()
}

func (a *App) click(id string) {
	switch {
	case id == "back":
		a.back()
	case id == "mute":
		a.snd.Mute = !a.snd.Mute
		a.cfg.Mute = a.snd.Mute
		a.saveCfg()
	case id == "pos":
		a.cycleSpot()
	case id == "close":
		a.quit = true
	case id == "pong":
		a.go_(SPong)
	case id == "perfil":
		a.go_(SProfile)
	case id == "pfname":
		a.sel = 4
	case strings.HasPrefix(id, "pf:"):
		d := 1
		if strings.HasSuffix(id, ":-1") {
			d = -1
		}
		a.profChange(int(id[3]-'0'), d)
	case id == "local":
		a.startGame(ModeLocal)
	case id == "cpu":
		a.startGame(ModeCPU)
	case id == "host":
		a.hostRoom()
	case id == "join":
		a.openJoin()
	case id == "ipok":
		if a.joinStart.IsZero() {
			a.joinInput()
		}
	case strings.HasPrefix(id, "room:"):
		if addr, err := net.ResolveUDPAddr("udp4", strings.TrimPrefix(id, "room:")); err == nil {
			a.joinAddr(addr)
		}
	}
}

// ---------- Update ----------

func pressed(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if inpututil.IsKeyJustPressed(k) {
			return true
		}
	}
	return false
}

func keyDir(up, down ebiten.Key) float64 {
	d := 0.0
	if ebiten.IsKeyPressed(up) {
		d--
	}
	if ebiten.IsKeyPressed(down) {
		d++
	}
	return d
}

func (a *App) Update() error {
	a.tick++
	if !a.placed {
		a.placeWindow()
		a.placed = true
	}
	ix, iy := ebiten.CursorPosition()
	a.cx, a.cy = float64(ix), float64(iy)

	// atajos generales
	cmd := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	if cmd && pressed(ebiten.KeyQ, ebiten.KeyW) {
		a.quit = true
	}
	if pressed(ebiten.KeyTab) {
		a.panic = !a.panic
	}
	if a.tick%20 == 0 { // el gráfico falso de CPU se mueve solo
		v := clamp(a.cpuHist[len(a.cpuHist)-1]+(rand.Float64()-.5)*8, 4, 38)
		a.cpuHist = append(a.cpuHist[1:], v)
	}
	typing := a.scene == SJoin || (a.scene == SProfile && a.sel == 4)
	if !a.panic {
		if pressed(ebiten.KeyEscape) {
			a.back()
		}
		if !typing && pressed(ebiten.KeyM) {
			a.click("mute")
		}
	}
	if pressed(ebiten.KeyEqual, ebiten.KeyNumpadAdd) {
		a.resize(1)
	}
	if pressed(ebiten.KeyMinus, ebiten.KeyNumpadSubtract) {
		a.resize(-1)
	}

	// ratón: botones o arrastrar la ventana
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		hit := false
		for _, b := range a.buttons() {
			if b.has(a.cx, a.cy) {
				a.click(b.id)
				hit = true
				break
			}
		}
		if !hit && a.inDragZone() {
			a.dragging, a.dragX, a.dragY = true, a.cx, a.cy
		}
	}
	if a.dragging {
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			ws, _ := ebiten.WindowSize()
			sc := float64(ws) / W
			dx, dy := int(math.Round((a.cx-a.dragX)*sc)), int(math.Round((a.cy-a.dragY)*sc))
			if dx != 0 || dy != 0 {
				x, y := ebiten.WindowPosition()
				ebiten.SetWindowPosition(x+dx, y+dy)
			}
		} else {
			a.dragging = false
			a.cfg.X, a.cfg.Y = ebiten.WindowPosition()
			a.cfg.HasPos = true
			a.saveCfg()
		}
	}

	if a.panic {
		if a.scene == SGame {
			a.updateGame() // la partida sigue conectada, pero en pausa
		}
	} else {
		switch a.scene {
		case SMenu:
			if pressed(ebiten.KeyArrowDown, ebiten.KeyS, ebiten.KeyArrowUp, ebiten.KeyW) {
				a.sel = (a.sel + 1) % len(menuSel)
			}
			if pressed(ebiten.KeyEnter, ebiten.KeySpace) {
				a.click(menuRows[menuSel[a.sel%len(menuSel)]].id)
			}
		case SPong:
			if pressed(ebiten.KeyArrowDown, ebiten.KeyS) {
				a.sel = (a.sel + 1) % len(modeItems)
			}
			if pressed(ebiten.KeyArrowUp, ebiten.KeyW) {
				a.sel = (a.sel + len(modeItems) - 1) % len(modeItems)
			}
			for i, k := range []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4} {
				if pressed(k) {
					a.click(modeItems[i].id)
				}
			}
			if pressed(ebiten.KeyEnter, ebiten.KeySpace) {
				a.click(modeItems[a.sel].id)
			}
		case SProfile:
			a.updateProfile()
		case SHost:
			if a.nw != nil && a.nw.Connected() {
				a.startGame(ModeHost)
			}
		case SJoin:
			a.updateJoin()
		case SGame:
			a.updateGame()
		}
	}

	a.lastCX, a.lastCY = a.cx, a.cy
	if st, _ := a.upd.Status(); st == "listo" && !a.panic && (a.scene == SMenu || a.scene == SPong || a.scene == SProfile) {
		if a.updAt == 0 {
			a.updAt = a.tick + 90 // se muestra el aviso un momento y se reinicia
		} else if a.tick >= a.updAt && a.upd.Apply() {
			a.quit = true
		}
	}
	if a.quit {
		a.nw.Close()
		a.saveCfg()
		return ebiten.Termination
	}
	return nil
}

func (a *App) inDragZone() bool {
	if a.cx < 0 || a.cy < 0 || a.cx >= W || a.cy >= H {
		return false
	}
	if a.scene == SGame && !a.panic {
		return a.cy < 16 && a.cx > 20 && a.cx < 190
	}
	return true // en los menús se arrastra desde cualquier parte que no sea un botón
}

func (a *App) updateProfile() {
	if pressed(ebiten.KeyArrowDown) {
		a.sel = (a.sel + 1) % len(profLabels)
	}
	if pressed(ebiten.KeyArrowUp) {
		a.sel = (a.sel + len(profLabels) - 1) % len(profLabels)
	}
	if a.sel < 4 {
		if pressed(ebiten.KeyArrowLeft) {
			a.profChange(a.sel, -1)
		}
		if pressed(ebiten.KeyArrowRight) {
			a.profChange(a.sel, 1)
		}
		return
	}
	changed := false
	for _, r := range ebiten.AppendInputChars(nil) {
		if n := cleanName(a.cfg.Av.Name + string(r)); n != a.cfg.Av.Name {
			a.cfg.Av.Name, changed = n, true
		}
	}
	if pressed(ebiten.KeyBackspace) && a.cfg.Av.Name != "" {
		a.cfg.Av.Name, changed = a.cfg.Av.Name[:len(a.cfg.Av.Name)-1], true
	}
	if pressed(ebiten.KeyEnter) {
		a.sel = 0
	}
	if changed {
		a.saveCfg()
	}
}

func (a *App) updateJoin() {
	for _, r := range ebiten.AppendInputChars(nil) {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		if (r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r == '.') && len(a.ipInput) < 15 {
			a.ipInput += string(r)
			if a.joinStart.IsZero() {
				a.status = ""
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.ipInput) > 0 {
		a.ipInput = a.ipInput[:len(a.ipInput)-1]
	}
	rooms := a.nw.Rooms()
	if len(rooms) > 0 && a.joinStart.IsZero() && os.Getenv("MJ_AUTOJOIN") == "lan" { // solo para pruebas
		a.joinAddr(rooms[0].Addr)
	}
	if code := os.Getenv("MJ_AUTOJOIN"); len(code) == 4 && a.joinStart.IsZero() && a.status == "" { // solo para pruebas
		a.ipInput = code
		a.joinInput()
	}
	if pressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter) && a.joinStart.IsZero() {
		if a.ipInput == "" && len(rooms) > 0 {
			a.joinAddr(rooms[0].Addr)
		} else {
			a.joinInput()
		}
	}
	if a.joinStart.IsZero() {
		return
	}
	if a.nw.Connected() {
		a.joinStart = time.Time{}
		a.startGame(ModeGuest)
		return
	}
	fail := ""
	switch a.nw.RTCStatus() {
	case NetNoInternet:
		fail = "SIN INTERNET"
	case NetNoRoom:
		fail = "NO EXISTE ESA SALA"
	case NetFailed:
		fail = "NO SE PUDO CONECTAR"
	}
	if fail == "" && time.Since(a.joinStart) > 30*time.Second {
		fail = "NO RESPONDE"
	}
	if fail != "" {
		a.joinStart = time.Time{}
		a.nw.Close()
		a.nw = NewSearcher()
		a.status, a.statusErr = fail, true
	}
}

func (a *App) myInput(side int) Input {
	var in Input
	if a.panic {
		return in
	}
	if a.mode == ModeLocal {
		if side == 1 {
			in.Dir = keyDir(ebiten.KeyW, ebiten.KeyS)
		} else {
			in.Dir = keyDir(ebiten.KeyArrowUp, ebiten.KeyArrowDown)
		}
	} else {
		in.Dir = clamp(keyDir(ebiten.KeyW, ebiten.KeyS)+keyDir(ebiten.KeyArrowUp, ebiten.KeyArrowDown), -1, 1)
	}
	if a.ptrActive && (a.mode != ModeLocal || a.ptrSide == side) {
		in.Target, in.HasTarget = a.ptrY, true
	}
	if botMode && a.pong != nil { // pruebas: sigue la bola sola
		in.Target, in.HasTarget = a.pong.Y+BS/2, true
	}
	return in
}

func (a *App) updatePointer() {
	inside := a.cx >= 0 && a.cy >= 16 && a.cx < W && a.cy < H
	if pressed(ebiten.KeyW, ebiten.KeyS, ebiten.KeyArrowUp, ebiten.KeyArrowDown) {
		a.ptrActive = false
	}
	if a.dragging || a.panic {
		return
	}
	if a.mode == ModeLocal { // mismo PC: solo arrastrando; cada mitad mueve su paleta
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && inside {
			a.ptrActive, a.ptrY = true, a.cy
			a.ptrSide = 1
			if a.cx >= W/2 {
				a.ptrSide = 2
			}
		} else if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			a.ptrActive = false
		}
		return
	}
	if inside && (a.cx != a.lastCX || a.cy != a.lastCY) {
		a.ptrActive, a.ptrY = true, a.cy
	}
}

func (a *App) updateGame() {
	p := a.pong
	a.updatePointer()

	act := !a.panic && (pressed(ebiten.KeySpace, ebiten.KeyEnter) ||
		(inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && a.cy > 16 && (p.Ph == PhOver || p.Paused)))
	if !a.panic && pressed(ebiten.KeyP) && (a.mode == ModeLocal || a.mode == ModeCPU) && p.Ph != PhOver {
		p.Paused = !p.Paused
	} else if act {
		switch {
		case p.Paused && (a.mode == ModeLocal || a.mode == ModeCPU):
			p.Paused = false
		case p.Ph == PhDC:
			a.back()
			return
		case p.Ph == PhOver && a.mode == ModeGuest:
			a.rematch++
		case p.Ph == PhOver && a.mode != ModeGuest:
			p.Reset()
		}
	}
	var panicFlag uint8
	if a.panic {
		panicFlag = 1
	}

	switch a.mode {
	case ModeLocal, ModeCPU:
		if a.panic {
			p.Paused = true // al volver queda en pausa: P o Espacio para seguir
		}
		in2 := p.CPUInput()
		if a.mode == ModeLocal {
			in2 = a.myInput(2)
		}
		p.Step(a.myInput(1), &in2, Dt)
	case ModeHost:
		if b, rm, ok := a.nw.GuestPaddle(); ok {
			a.targetB = clamp(b, 0, H-PH)
			p.B += (a.targetB - p.B) * .35 // llega a saltos por el servidor: se suaviza
			if rm != a.lastRematch {
				a.lastRematch = rm
				if p.Ph == PhOver {
					p.Reset()
				}
			}
		}
		if a.nw.Lost() {
			p.Ph = PhDC
		}
		p.Paused = a.panic || a.nw.GuestPanic() // si alguno se esconde, se pausa para los dos
		if p.Paused {
			panicFlag = 1
		}
		p.RemoteRight, p.Grace = true, .45
		if a.nw.Via() == "relay" {
			p.Grace = .9 // por el servidor los avisos tardan más
		}
		if h, ok := a.nw.TakeHit(); ok && h.Idx == p.Ev[EvHit2] && int(h.Sum) == p.S1+p.S2 {
			if p.RemoteHit(h.Y, h.B) && botMode {
				logf("prueba: golpe remoto aplicado (%.2fs después)", p.since)
			}
		}
		if botMode && a.tick%600 == 0 {
			logf("prueba: marcador %d-%d golpes %d/%d", p.S1, p.S2, p.Ev[EvHit1], p.Ev[EvHit2])
		}
		p.Step(a.myInput(1), nil, Dt)
		a.nw.SendState(p, panicFlag)
	case ModeGuest:
		if st, ok := a.nw.State(); ok {
			if a.hitPending && (st.Ev[EvHit2] != a.hitMsg.Idx || int(st.S1+st.S2) != int(a.hitMsg.Sum) || a.tick-a.hitAt > 90) {
				a.hitPending = false // el anfitrión ya lo aplicó (o ya no sirve)
			}
			if st.Seq != a.lastSeq || !a.evReady { // paquete nuevo del anfitrión
				a.lastSeq = st.Seq
				if r := float64(NowMs()-st.Echo-st.Hold) / 1000; r < 2 { // demora de ida y vuelta
					if a.lat == 0 {
						a.lat = r / 2
					}
					a.lat = a.lat*.8 + r/2*.2
				}
				if !a.hitPending { // mientras espero confirmación, sigo mostrando mi rebote
					// el paquete muestra el pasado: adelanto la bola lo que tardó en llegar
					tx, ty, vx, vy := advanceBall(float64(st.X), float64(st.Y), float64(st.VX), float64(st.VY), a.lat)
					if p.Ph != PhPlay || st.Ph != PhPlay || math.Abs(tx-p.X) > 30 || math.Abs(ty-p.Y) > 30 || (vx > 0) != (p.VX > 0) {
						p.X, p.Y, a.corrX, a.corrY = tx, ty, 0, 0
					} else {
						a.corrX, a.corrY = tx-p.X, ty-p.Y // se corrige de a poco, sin saltos
					}
					p.VX, p.VY = vx, vy
				}
				p.Tm = float64(st.Tm)
				p.S1, p.S2, p.Ph, p.Winner, p.Ev = int(st.S1), int(st.S2), st.Ph, int(st.Winner), st.Ev
				p.Paused = st.Flags&1 != 0
				a.targetA = float64(st.A)
				if !a.evReady {
					a.lastEv, a.evReady, p.A = p.Ev, true, a.targetA
				}
			}
			if p.Ph == PhPlay && !p.Paused { // entre paquetes: la bola sigue su camino
				p.X, p.Y, p.VX, p.VY = advanceBall(p.X+a.corrX*.25, p.Y+a.corrY*.25, p.VX, p.VY, Dt)
				a.corrX, a.corrY = a.corrX*.75, a.corrY*.75
			}
			// mis golpes los decido yo, con lo que veo en mi pantalla
			if !a.hitPending && p.Ph == PhPlay && !p.Paused && p.VX > 0 &&
				p.X+BS >= W-PX-PW && p.X <= W-PX && p.Y+BS >= p.B && p.Y <= p.B+PH {
				a.hitPending, a.hitAt = true, a.tick
				a.hitMsg = HitPkt{Idx: p.Ev[EvHit2], Sum: uint8(p.S1 + p.S2), Y: p.Y, B: p.B}
				saved := p.Ev
				p.hit(2) // rebote inmediato en mi pantalla
				p.Ev = saved
				a.snd.Play(EvHit2)
				a.pulse2 = 1
				a.skipHit2 = a.hitMsg.Idx + 1 // que no suene de nuevo al confirmarse
			}
			if d := a.tick - a.hitAt; a.hitPending && (d%3 == 0 && !a.nw.Relayed() || d == 0 || d == 10 || d == 25 || d == 45) {
				a.nw.SendHit(a.hitMsg) // por el servidor, pocas veces (tiene límite de mensajes)
			}
			p.A += (a.targetA - p.A) * .5
		}
		if a.nw.Lost() {
			p.Ph = PhDC
		}
		if p.Ph != PhDC {
			if !p.Paused {
				p.B = movePaddle(p.B, a.myInput(2), Dt) // mi paleta la muevo yo: se siente instantánea
			}
			a.nw.SendInput(p.B, a.rematch, panicFlag)
		}
	}
	if (a.mode == ModeHost || a.mode == ModeGuest) && (a.tick%20 == 0 && !a.nw.Relayed() || a.tick%180 == 0) {
		a.nw.SendAvatar(a.cfg.Av)
	}

	// sonidos y efectos de lo que pasó
	for i := 0; i < EvCount; i++ {
		if i == EvHit2 && a.mode == ModeGuest && p.Ev[i] == a.skipHit2 && p.Ev[i] != a.lastEv[i] {
			continue // ya sonó cuando golpeé
		}
		if p.Ev[i] != a.lastEv[i] && !a.panic {
			a.snd.Play(i)
			switch i {
			case EvPoint:
				a.shake, a.flash = .25, .35
			case EvHit1:
				a.pulse1 = 1
			case EvHit2:
				a.pulse2 = 1
			}
		}
	}
	a.lastEv = p.Ev

	if p.Ph == PhPlay && !p.Paused {
		a.trail = append(a.trail, [2]float64{p.X, p.Y})
		if len(a.trail) > 7 {
			a.trail = a.trail[1:]
		}
	} else if !p.Paused {
		a.trail = a.trail[:0]
	}
	a.shake = math.Max(0, a.shake-Dt)
	a.flash = math.Max(0, a.flash-Dt*2)
	a.pulse1 = math.Max(0, a.pulse1-Dt*4)
	a.pulse2 = math.Max(0, a.pulse2-Dt*4)
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

func (a *App) hoverAny() bool {
	for _, b := range a.buttons() {
		if a.hov(b) && b.id != "back" && b.id != "mute" && b.id != "pos" && b.id != "close" {
			return true
		}
	}
	return false
}

func main() {
	a := NewApp()
	if d := os.Getenv("MJ_DEMO"); d != "" { // para pruebas: arranca directo en una pantalla
		a.click(d)
	}
	if os.Getenv("MJ_PANIC") != "" {
		a.panic = true
	}
	sz := winSizes[a.cfg.Scale]
	ebiten.SetWindowTitle("Idle")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetWindowSize(sz, sz)
	ebiten.SetWindowIcon([]image.Image{appIcon(16), appIcon(32), appIcon(64)})
	a.placeWindow()
	if err := ebiten.RunGame(a); err != nil {
		log.Fatal(err)
	}
}
