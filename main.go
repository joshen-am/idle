package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ---------- colores y letra ----------

func hex(h uint32) color.NRGBA { return color.NRGBA{uint8(h >> 16), uint8(h >> 8), uint8(h), 255} }
func fade(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(clamp(a, 0, 1) * 255)
	return c
}

var (
	colBg    = hex(0x07080f)
	colPanel = hex(0x0c0f1f)
	colCard  = hex(0x12162c)
	colLine  = hex(0x232a52)
	colField = hex(0x1a1f3d)
	colInk   = hex(0xe8ecff)
	colDim   = hex(0x6b72a8)
	colP1    = hex(0x34e7ff)
	colP2    = hex(0xff4fa3)
	colBall  = hex(0xffe14f)
	colOk    = hex(0x7dff8a)
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

var (
	icBack  = []px{{4, 1, 1, 1}, {3, 2, 1, 1}, {2, 3, 1, 2}, {3, 5, 1, 1}, {4, 6, 1, 1}}
	icSpk   = []px{{1, 3, 1, 2}, {2, 2, 1, 4}, {3, 1, 1, 6}}
	icWaves = []px{{5, 2, 1, 1}, {6, 3, 1, 2}, {5, 5, 1, 1}}
	icPos   = []px{{0, 0, 8, 1}, {0, 7, 8, 1}, {0, 1, 1, 6}, {7, 1, 1, 6}, {4, 4, 3, 3}}
	icClose = []px{{1, 1, 1, 1}, {2, 2, 1, 1}, {3, 3, 2, 2}, {5, 2, 1, 1}, {6, 1, 1, 1}, {2, 5, 1, 1}, {1, 6, 1, 1}, {5, 5, 1, 1}, {6, 6, 1, 1}}
)

// ---------- configuración guardada (sonido, tamaño, posición) ----------

type Config struct {
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
	return filepath.Join(d, "MiniJuegos", "config.json")
}

func loadCfg() Config {
	var c Config
	if b, err := os.ReadFile(cfgPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	if c.Scale < 0 || c.Scale >= len(winSizes) {
		c.Scale = 0
	}
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

	// juego
	mode                         Mode
	pong                         *Pong
	lastEv                       [EvCount]uint8
	evReady                      bool
	rematch                      uint8
	lastRematch                  uint8
	lastSeq                      uint16
	targetA                      float64
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
	go a.upd.Check()
	a.snd.Mute = a.cfg.Mute
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
	case SPong:
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
	if a.scene != SMenu {
		bs = append(bs, btn{4, 4, 12, 12, "back"})
	}
	return append(bs, btn{196, 4, 12, 12, "mute"}, btn{210, 4, 12, 12, "pos"}, btn{224, 4, 12, 12, "close"})
}

var modeItems = []struct {
	id, title, sub string
	c              color.NRGBA
}{
	{"local", "MISMO PC", "W/S CONTRA FLECHAS", colP1},
	{"cpu", "VS CPU", "PRACTICA SOLO", colBall},
	{"host", "CREAR SALA", "TE DA UN CODIGO", colP1},
	{"join", "UNIRSE", "ESCRIBES EL CODIGO", colP2},
}

func (a *App) buttons() []btn {
	bs := a.topButtons()
	switch a.scene {
	case SMenu:
		bs = append(bs, btn{8, 52, 108, 86, "pong"})
	case SPong:
		for i, m := range modeItems {
			bs = append(bs, btn{10, 36 + float64(i)*46, 220, 40, m.id})
		}
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
	if pressed(ebiten.KeyEscape) {
		a.back()
	}
	typing := a.scene == SJoin
	if !typing && pressed(ebiten.KeyM) {
		a.click("mute")
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

	switch a.scene {
	case SMenu:
		if pressed(ebiten.KeyEnter, ebiten.KeySpace) {
			a.go_(SPong)
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
	case SHost:
		if a.nw != nil && a.nw.Connected() {
			a.startGame(ModeHost)
		}
	case SJoin:
		a.updateJoin()
	case SGame:
		a.updateGame()
	}

	a.lastCX, a.lastCY = a.cx, a.cy
	if st, _ := a.upd.Status(); st == "listo" && (a.scene == SMenu || a.scene == SPong) {
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
	if a.scene == SGame {
		return a.cy < 16 && a.cx > 20 && a.cx < 190
	}
	return true // en los menús se arrastra desde cualquier parte que no sea un botón
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
	return in
}

func (a *App) updatePointer() {
	inside := a.cx >= 0 && a.cy >= 16 && a.cx < W && a.cy < H
	if pressed(ebiten.KeyW, ebiten.KeyS, ebiten.KeyArrowUp, ebiten.KeyArrowDown) {
		a.ptrActive = false
	}
	if a.dragging {
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

	act := pressed(ebiten.KeySpace, ebiten.KeyEnter) ||
		(inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && a.cy > 16 && (p.Ph == PhOver || p.Paused))
	if pressed(ebiten.KeyP) && (a.mode == ModeLocal || a.mode == ModeCPU) && p.Ph != PhOver {
		p.Paused = !p.Paused
	} else if act {
		switch {
		case p.Paused:
			p.Paused = false
		case p.Ph == PhDC:
			a.back()
			return
		case p.Ph == PhOver && a.mode == ModeGuest:
			a.rematch++
		case p.Ph == PhOver:
			p.Reset()
		}
	}

	switch a.mode {
	case ModeLocal:
		in2 := a.myInput(2)
		p.Step(a.myInput(1), &in2, Dt)
	case ModeCPU:
		in2 := p.CPUInput()
		p.Step(a.myInput(1), &in2, Dt)
	case ModeHost:
		if b, rm, ok := a.nw.GuestPaddle(); ok {
			p.B = clamp(b, 0, H-PH)
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
		p.Step(a.myInput(1), nil, Dt)
		a.nw.SendState(p)
	case ModeGuest:
		if st, ok := a.nw.State(); ok {
			if st.Seq != a.lastSeq || !a.evReady { // paquete nuevo del anfitrión
				a.lastSeq = st.Seq
				p.X, p.Y, p.VX, p.VY, p.Tm = float64(st.X), float64(st.Y), float64(st.VX), float64(st.VY), float64(st.Tm)
				p.S1, p.S2, p.Ph, p.Winner, p.Ev = int(st.S1), int(st.S2), st.Ph, int(st.Winner), st.Ev
				a.targetA = float64(st.A)
				if !a.evReady {
					a.lastEv, a.evReady, p.A = p.Ev, true, a.targetA
				}
			} else if p.Ph == PhPlay { // entre paquetes: la bola sigue su camino (se ve fluido aunque haya demora)
				p.X = clamp(p.X+p.VX*Dt, PX+PW, W-PX-PW-BS)
				p.Y += p.VY * Dt
				if p.Y < 0 || p.Y > H-BS {
					p.Y, p.VY = clamp(p.Y, 0, H-BS), -p.VY
				}
			}
			p.A += (a.targetA - p.A) * .5
		}
		if a.nw.Lost() {
			p.Ph = PhDC
		}
		if p.Ph != PhDC {
			p.B = movePaddle(p.B, a.myInput(2), Dt) // mi paleta la muevo yo: se siente instantánea
			a.nw.SendInput(p.B, a.rematch)
		}
	}

	// sonidos y efectos de lo que pasó
	for i := 0; i < EvCount; i++ {
		if p.Ev[i] != a.lastEv[i] {
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

	if p.Ph == PhPlay {
		a.trail = append(a.trail, [2]float64{p.X, p.Y})
		if len(a.trail) > 7 {
			a.trail = a.trail[1:]
		}
	} else {
		a.trail = a.trail[:0]
	}
	a.shake = math.Max(0, a.shake-Dt)
	a.flash = math.Max(0, a.flash-Dt*2)
	a.pulse1 = math.Max(0, a.pulse1-Dt*4)
	a.pulse2 = math.Max(0, a.pulse2-Dt*4)
}

// ---------- Draw ----------

func (a *App) Layout(int, int) (int, int) { return W, H }

func (a *App) Draw(s *ebiten.Image) {
	if a.scene == SGame {
		a.drawGame(s)
	} else {
		s.Fill(colPanel)
		switch a.scene {
		case SMenu:
			a.drawMenu(s)
		case SPong:
			a.drawModes(s)
		case SHost:
			a.drawHost(s)
		case SJoin:
			a.drawJoin(s)
		}
		a.drawTopBar(s)
		a.drawUpdate(s)
	}
	frame(s, 0, 0, W, H, colLine) // borde para que se vea el contorno de la ventana
	shot(s, a.tick)
}

func (a *App) drawUpdate(s *ebiten.Image) {
	st, v := a.upd.Status()
	msg := ""
	switch st {
	case "descargando":
		msg = "BAJANDO V" + v + dots(a.tick)
	case "listo":
		msg = "ACTUALIZANDO A V" + v
	}
	if msg == "" {
		if a.scene == SMenu {
			txt(s, "V"+version, W-6, H-11, 8, colLine, 1)
		}
		return
	}
	rect(s, 0, H-14, W, 14, colBg)
	rect(s, 0, H-15, W, 1, colLine)
	txt(s, msg, W/2, H-11, 8, colOk, 0)
}

func (a *App) hov(b btn) bool { return b.has(a.cx, a.cy) && !a.dragging }

func (a *App) drawTopBar(s *ebiten.Image) {
	rect(s, 0, 0, W, 20, colBg)
	rect(s, 0, 20, W, 1, colLine)
	title := map[Scene]string{SMenu: "", SPong: "PONG", SHost: "CREAR SALA", SJoin: "UNIRSE"}[a.scene]
	cx := 100.0
	if a.scene != SMenu {
		cx = 106
	}
	txt(s, title, cx, 6, 8, colInk, 0)
	for _, b := range a.topButtons() {
		a.drawIconBtn(s, b, 1)
	}
}

func (a *App) drawIconBtn(s *ebiten.Image, b btn, alpha float64) {
	h := a.hov(b)
	c := colInk
	if h {
		c, alpha = colP1, 1
	}
	rect(s, b.x, b.y, b.w, b.h, fade(colBg, alpha*.8))
	frame(s, b.x, b.y, b.w, b.h, fade(map[bool]color.NRGBA{false: colLine, true: colP1}[h], alpha))
	ox, oy := b.x+2, b.y+2
	switch b.id {
	case "back":
		pix(s, ox, oy, 1, icBack, fade(c, alpha))
	case "mute":
		if a.snd.Mute {
			pix(s, ox, oy, 1, icSpk, fade(colDim, alpha))
		} else {
			pix(s, ox, oy, 1, icSpk, fade(c, alpha))
			pix(s, ox, oy, 1, icWaves, fade(c, alpha))
		}
	case "pos":
		pix(s, ox, oy, 1, icPos, fade(c, alpha))
	case "close":
		pix(s, ox, oy, 1, icClose, fade(c, alpha))
	}
}

func card(s *ebiten.Image, b btn, active bool) {
	if active {
		rect(s, b.x+1, b.y+1, b.w, b.h, colLine)
		b.x, b.y = b.x-1, b.y-1
	}
	rect(s, b.x, b.y, b.w, b.h, colCard)
	c := colLine
	if active {
		c = colP1
	}
	frame(s, b.x, b.y, b.w, b.h, c)
	frame(s, b.x+1, b.y+1, b.w-2, b.h-2, c)
}

func (a *App) drawMenu(s *ebiten.Image) {
	txt(s, "MINI", 72, 30, 16, colP1, 0)
	txt(s, "JUEGOS", 168, 30, 16, colP2, 0)
	tiles := []struct {
		name, name2 string
		icon        func(*ebiten.Image, float64, float64)
		on          bool
	}{
		{"PONG", "", iconPong, true},
		{"SNAKE", "DUELO", iconSnake, false},
		{"TRES EN", "RAYA", iconTTT, false},
		{"AIR", "HOCKEY", iconHockey, false},
	}
	for i, t := range tiles {
		b := btn{8 + float64(i%2)*116, 52 + float64(i/2)*94, 108, 86, ""}
		active := t.on && (a.hov(b) || a.tick < 0)
		card(s, b, active)
		ox, oy := b.x, b.y
		if active {
			ox, oy = ox-1, oy-1
		}
		t.icon(s, ox+38, oy+10)
		txt(s, t.name, ox+54, oy+48, 8, colInk, 0)
		if t.name2 != "" {
			txt(s, t.name2, ox+54, oy+58, 8, colInk, 0)
		}
		tag, tc := "JUGAR", colOk
		if !t.on {
			tag, tc = "PRONTO", colDim
		}
		txt(s, tag, ox+54, oy+72, 8, tc, 0)
		if !t.on {
			rect(s, b.x, b.y, b.w, b.h, fade(colPanel, .55))
		}
	}
}

func iconPong(s *ebiten.Image, x, y float64) {
	pix(s, x, y, 2, []px{{2, 3, 2, 6}}, colP1)
	pix(s, x, y, 2, []px{{12, 7, 2, 6}}, colP2)
	pix(s, x, y, 2, []px{{7, 7, 2, 2}}, colBall)
	pix(s, x, y, 2, []px{{5, 9, 1, 1}}, fade(colBall, .5))
	pix(s, x, y, 2, []px{{3.5, 10.5, 1, 1}}, fade(colBall, .25))
}
func iconSnake(s *ebiten.Image, x, y float64) {
	pix(s, x, y, 2, []px{{2, 3, 6, 2}, {6, 5, 2, 4}, {6, 9, 6, 2}}, colOk)
	pix(s, x, y, 2, []px{{12, 3, 2, 2}}, colP2)
}
func iconTTT(s *ebiten.Image, x, y float64) {
	pix(s, x, y, 2, []px{{5, 1, 1, 14}, {10, 1, 1, 14}, {1, 5, 14, 1}, {1, 10, 14, 1}}, colLine)
	pix(s, x, y, 2, []px{{2, 2, 2, 2}, {12, 12, 2, 2}}, colP1)
	pix(s, x, y, 2, []px{{7, 7, 2, 2}}, colP2)
}
func iconHockey(s *ebiten.Image, x, y float64) {
	pix(s, x, y, 2, []px{{1, 7, 14, 1}}, colLine)
	pix(s, x, y, 2, []px{{6, 1, 4, 3}}, colP1)
	pix(s, x, y, 2, []px{{6, 12, 4, 3}}, colP2)
	pix(s, x, y, 2, []px{{9, 8, 2, 2}}, colBall)
}

func (a *App) drawModes(s *ebiten.Image) {
	txt(s, "PRIMERO A 7 GANA", W/2, 24, 8, colDim, 0)
	for i, m := range modeItems {
		b := btn{10, 36 + float64(i)*46, 220, 40, m.id}
		active := a.hov(b) || (a.sel == i && !a.hoverAny())
		card(s, b, active)
		ox, oy := b.x, b.y
		if active {
			ox, oy = ox-1, oy-1
		}
		if m.id == "local" {
			rect(s, ox+10, oy+12, 4, 8, colP1)
			rect(s, ox+14, oy+12, 4, 8, colP2)
		} else {
			rect(s, ox+10, oy+12, 8, 8, m.c)
		}
		txt(s, m.title, ox+28, oy+10, 8, colInk, -1)
		txt(s, m.sub, ox+28, oy+24, 8, colDim, -1)
	}
	txt(s, "ESC VOLVER  +/- ZOOM", W/2, 226, 8, colDim, 0)
}

func (a *App) hoverAny() bool {
	for _, b := range a.buttons() {
		if a.hov(b) && b.id != "back" && b.id != "mute" && b.id != "pos" && b.id != "close" {
			return true
		}
	}
	return false
}

func dots(t int) string { return strings.Repeat(".", (t/18)%4) }

func (a *App) drawHost(s *ebiten.Image) {
	txt(s, "TU CODIGO", W/2, 32, 8, colDim, 0)
	rect(s, 60, 46, 120, 36, colBg)
	frame(s, 60, 46, 120, 36, colLine)
	code := "...."
	st := NetConnecting
	if a.nw != nil {
		code, st = a.nw.Code, a.nw.RTCStatus()
	}
	if st == NetConnecting {
		code = "...."
	}
	txt(s, strings.Join(strings.Split(code, ""), " "), W/2+1, 56, 16, colBall, 0)
	txt(s, "DISELO A TU AMIGO", W/2, 94, 8, colInk, 0)
	txt(s, "ESPERANDO"+dots(a.tick), 84, 116, 8, colDim, -1)

	switch st {
	case NetOnline:
		txt(s, "INTERNET OK", W/2, 146, 8, colOk, 0)
	case NetNoInternet:
		txt(s, "SIN INTERNET:", W/2, 140, 8, colP2, 0)
		txt(s, "SOLO RED LOCAL", W/2, 152, 8, colP2, 0)
	default:
		txt(s, "CONECTANDO"+dots(a.tick), 80, 146, 8, colDim, -1)
	}
	txt(s, "EN EL OTRO PC:", W/2, 178, 8, colDim, 0)
	txt(s, "PONG > UNIRSE", W/2, 192, 8, colInk, 0)
	txt(s, "IP "+LocalIP(), W/2, 220, 8, colDim, 0)
}

func (a *App) drawJoin(s *ebiten.Image) {
	txt(s, "ESCRIBE EL CODIGO", W/2, 30, 8, colDim, 0)
	rect(s, 30, 42, 132, 30, colBg)
	frame(s, 30, 42, 132, 30, colLine)
	cur := ""
	if (a.tick/30)%2 == 0 && a.joinStart.IsZero() {
		cur = "_"
	}
	if len(a.ipInput) <= 4 && !strings.Contains(a.ipInput, ".") {
		txt(s, strings.Join(strings.Split(a.ipInput+cur, ""), " "), 40, 50, 16, colBall, -1)
	} else {
		txt(s, a.ipInput+cur, 36, 54, 8, colBall, -1)
	}
	ok := btn{168, 42, 42, 30, "ipok"}
	c := colLine
	if a.hov(ok) {
		c = colP1
	}
	rect(s, ok.x, ok.y, ok.w, ok.h, colCard)
	frame(s, ok.x, ok.y, ok.w, ok.h, c)
	txt(s, "OK", ok.x+21, ok.y+11, 8, colInk, 0)
	if a.status != "" {
		col, st := colDim, a.status
		if a.statusErr {
			col = colP2
		} else {
			if a.nw != nil && a.nw.Via() == "relay" {
				st = "POR SERVIDOR"
			}
			st += dots(a.tick)
		}
		txt(s, st, W/2, 82, 8, col, 0)
	}

	txt(s, "O ELIGE UNA SALA CERCA:", W/2, 108, 8, colDim, 0)
	rooms := a.nw.Rooms()
	if len(rooms) == 0 {
		txt(s, "BUSCANDO"+dots(a.tick), 84, 132, 8, colLine, -1)
	}
	for i, r := range rooms {
		if i >= 3 {
			break
		}
		b := btn{10, 120 + float64(i)*30, 220, 26, ""}
		active := a.hov(b)
		card(s, b, active)
		ox, oy := b.x, b.y
		if active {
			ox, oy = ox-1, oy-1
		}
		rect(s, ox+8, oy+9, 8, 8, colP1)
		txt(s, r.Name, ox+24, oy+9, 8, colInk, -1)
		txt(s, r.Code, ox+212, oy+9, 8, colBall, 1)
	}
	txt(s, "(TAMBIEN SIRVE SU IP)", W/2, 222, 8, colLine, 0)
}

func glow(s *ebiten.Image, x, y, w, h float64, c color.NRGBA, pulse float64) {
	rect(s, x-2, y-2, w+4, h+4, fade(c, .14+pulse*.25))
	rect(s, x-4, y-4, w+8, h+8, fade(c, .07+pulse*.15))
	rect(s, x, y, w, h, c)
}

func (a *App) drawGame(s *ebiten.Image) {
	p := a.pong
	s.Fill(colBg)
	ox, oy := 0.0, 0.0
	if a.shake > 0 {
		ox, oy = math.Round((rand.Float64()-.5)*4), math.Round((rand.Float64()-.5)*4)
	}
	for y := 3.0; y < H; y += 10 {
		rect(s, W/2-1+ox, y+oy, 2, 5, colField)
	}
	txt(s, strconv.Itoa(p.S1), W/2-22+ox, 12+oy, 16, colP1, 1)
	txt(s, strconv.Itoa(p.S2), W/2+24+ox, 12+oy, 16, colP2, -1)

	glow(s, PX+ox, math.Round(p.A)+oy, PW, PH, colP1, a.pulse1)
	glow(s, W-PX-PW+ox, math.Round(p.B)+oy, PW, PH, colP2, a.pulse2)

	if p.Ph == PhPlay {
		for i, t := range a.trail {
			al := float64(i+1) / float64(len(a.trail)) * .35
			rect(s, math.Round(t[0]+1)+ox, math.Round(t[1]+1)+oy, BS-2, BS-2, fade(colBall, al))
		}
		glow(s, math.Round(p.X)+ox, math.Round(p.Y)+oy, BS, BS, colBall, 0)
	} else if p.Ph == PhServe && (a.tick/7)%2 == 0 {
		glow(s, W/2-2, H/2-2, BS, BS, colBall, 0)
	}
	if a.flash > 0 {
		rect(s, 0, 0, W, H, fade(colInk, a.flash*.5))
	}

	veil := func() { rect(s, 0, 84, W, 72, fade(colBg, .75)) }
	switch p.Ph {
	case PhCount:
		txt(s, strconv.Itoa(int(math.Max(1, math.Ceil(p.Tm)))), W/2+2, H/2-16, 32, colBall, 0)
		l := map[Mode][2]string{ModeLocal: {"W / S", "FLECHAS"}, ModeCPU: {"TU", "CPU"}, ModeHost: {"TU", "AMIGO"}, ModeGuest: {"AMIGO", "TU"}}[a.mode]
		txt(s, l[0], 18, H-22, 8, colP1, -1)
		txt(s, l[1], W-18, H-22, 8, colP2, 1)
	case PhOver:
		t, c := a.winnerText()
		veil()
		txt(s, t, W/2, 100, 16, c, 0)
		txt(s, fmt.Sprintf("%d - %d", p.S1, p.S2), W/2, 124, 8, colInk, 0)
		txt(s, "ESPACIO = REVANCHA", W/2, 142, 8, colDim, 0)
	case PhDC:
		veil()
		txt(s, "SE CORTO", W/2, 100, 16, colP2, 0)
		txt(s, "LA CONEXION", W/2, 124, 8, colInk, 0)
		txt(s, "ESPACIO = SALIR", W/2, 142, 8, colDim, 0)
	}
	if p.Paused {
		veil()
		txt(s, "PAUSA", W/2, 106, 16, colInk, 0)
		txt(s, "P = SEGUIR", W/2, 132, 8, colDim, 0)
	}
	for _, b := range a.topButtons() {
		a.drawIconBtn(s, b, .3)
	}
}

func (a *App) winnerText() (string, color.NRGBA) {
	w := a.pong.Winner
	switch a.mode {
	case ModeLocal:
		if w == 1 {
			return "GANA AZUL", colP1
		}
		return "GANA ROSA", colP2
	case ModeCPU:
		if w == 1 {
			return "¡GANASTE!", colBall
		}
		return "GANA CPU", colP2
	}
	me := 1
	if a.mode == ModeGuest {
		me = 2
	}
	if w == me {
		return "¡GANASTE!", colBall
	}
	return "PERDISTE", colP2
}

// ---------- ícono de la ventana ----------

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
	fill(0, 0, 16, 16, colBg)
	fill(2, 3, 2, 6, colP1)
	fill(12, 7, 2, 6, colP2)
	fill(7, 7, 2, 2, colBall)
	return out
}

// ---------- capturas para pruebas (MJ_SHOT_DIR) ----------

var shotDir = os.Getenv("MJ_SHOT_DIR")

func shot(s *ebiten.Image, tick int) {
	if shotDir == "" || tick%30 != 0 {
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	s.ReadPixels(img.Pix)
	f, err := os.Create(filepath.Join(shotDir, fmt.Sprintf("f%05d.png", tick)))
	if err == nil {
		png.Encode(f, img)
		f.Close()
	}
}

func main() {
	a := NewApp()
	if d := os.Getenv("MJ_DEMO"); d != "" { // para pruebas: arranca directo en una pantalla
		a.click(d)
	}
	sz := winSizes[a.cfg.Scale]
	ebiten.SetWindowTitle("Mini Juegos")
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
