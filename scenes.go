package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Tienda, ranking, diario y elección de rival (paneles al lado de la mascota).

// ---------- teclado y clics ----------

func (o *Overlay) sceneKey(k Key) {
	if k == KEsc {
		o.closePanel()
		return
	}
	switch o.scene {
	case SceneStore:
		n := catCount(storeCats[o.storeTab].key)
		switch k {
		case KLeft, KRight:
			o.storeTab = wrapInt(o.storeTab, map[Key]int{KLeft: -1, KRight: 1}[k], len(storeCats))
			o.storeSel = int(*slot(&o.cfg.Av, storeCats[o.storeTab].key))
		case KUp:
			o.storeSel = wrapInt(o.storeSel, -1, n)
		case KDown:
			o.storeSel = wrapInt(o.storeSel, 1, n)
		case KEnter, KSpace:
			o.buy()
		}
	case SceneDiary:
		switch k {
		case KLeft:
			o.diaryPage = max(0, o.diaryPage-1)
		case KRight, KEnter, KSpace:
			if o.diary != nil && o.diaryPage < len(o.diary.Pages)-1 {
				o.diaryPage++
			} else if k != KRight {
				o.closePanel()
			}
		}
	case SceneCPUPick:
		switch k {
		case KUp:
			o.sel = wrapInt(o.sel, -1, len(cpus))
		case KDown:
			o.sel = wrapInt(o.sel, 1, len(cpus))
		case KEnter, KSpace, KRight:
			o.startCPU(&cpus[o.sel])
		}
	case SceneBoard:
		if k == KEnter || k == KSpace {
			o.closePanel()
		}
	}
}

func wrapInt(v, d, n int) int { return ((v+d)%n + n) % n }

func (o *Overlay) sceneClick(id string) {
	var n int
	if i := strings.LastIndexByte(id, ':'); i >= 0 {
		fmt.Sscanf(id[i+1:], "%d", &n) // "st:i:3", "cp:2"...
	}
	switch {
	case strings.HasPrefix(id, "st:t:"):
		o.storeTab = n
		o.storeSel = int(*slot(&o.cfg.Av, storeCats[n].key))
	case strings.HasPrefix(id, "st:i:"):
		if o.storeSel == n {
			o.buy() // segundo clic en el mismo: comprar / ponérselo
		}
		o.storeSel = n
	case id == "st:buy":
		o.buy()
	case id == "dy:prev":
		o.diaryPage = max(0, o.diaryPage-1)
	case id == "dy:next" && o.diary != nil:
		o.diaryPage = min(len(o.diary.Pages)-1, o.diaryPage+1)
	case strings.HasPrefix(id, "cp:") && n < len(cpus):
		o.startCPU(&cpus[n])
	}
}

func (o *Overlay) say(s string) { o.toast, o.toastAt = s, time.Now() }

func (o *Overlay) buy() {
	key := storeCats[o.storeTab].key
	i := o.storeSel
	owned := o.cfg.Owns(key, i)
	if !o.cfg.Buy(key, i) {
		o.say(fmt.Sprintf("TE FALTAN %d ¤. ¡A JUGAR!", price(key, i)-o.cfg.Coins))
		return
	}
	if !owned {
		o.say("¡COMPRADO! YA LO LLEVAS PUESTO")
		o.pet.poke()
	}
	saveCfg(o.cfg)
}

// ---------- partidas: resultado y monedas ----------

// afterMatchFrame anota el resultado cuando termina una partida (una vez) y guarda al amigo.
func (o *Overlay) afterMatchFrame() {
	m := o.match
	if m.Online() && o.tick%60 == 0 {
		if p, ok := m.nw.PeerProfile(); ok {
			av, _ := m.nw.PeerAvatar()
			o.cfg.SeeFriend(p, av)
		}
	}
	if m.p.Ph != PhOver {
		if o.recorded {
			o.recorded, o.coinMsg, o.overLine = false, "", ""
		}
		return
	}
	if o.recorded {
		return
	}
	o.recorded = true
	won := m.p.Winner == m.MySide()
	me, them := m.p.S1, m.p.S2
	if m.MySide() == 2 {
		me, them = them, me
	}
	rec := MatchRec{Game: "pong", Won: won, Me: me, Them: them}
	if m.cpu != nil {
		rec.Opp, rec.OppName = m.cpu.ID, m.cpu.Name
		lines := cpuWinLines[m.cpu.ID]
		if won {
			lines = cpuLoseLines[m.cpu.ID]
		}
		o.overLine = fillName(lines[rand.Intn(len(lines))], o.cfg.Av.DisplayName())
	} else {
		rec.Opp, rec.OppName = "amigo", "AMIGO"
		if p, ok := m.nw.PeerProfile(); ok {
			rec.Opp, rec.OppName = p.ID, p.Name
		}
		if av, ok := m.nw.PeerAvatar(); ok && rec.OppName == "AMIGO" {
			rec.OppName = av.DisplayName()
		}
	}
	o.coinMsg = fmt.Sprintf("+%d ¤", o.cfg.Record(rec))
	m.prof = o.cfg.MyProfile()
	saveCfg(o.cfg)
	logf("partida: %s %d-%d contra %s (%s)", map[bool]string{true: "gane", false: "perdi"}[won], me, them, rec.OppName, o.coinMsg)
}

func fillName(s, b string) string {
	out := ""
	for i := 0; i < len(s); i++ {
		if i+3 <= len(s) && s[i:i+3] == "{B}" {
			out += b
			i += 2
			continue
		}
		out += s[i : i+1]
	}
	return out
}

// ---------- diario ----------

func (o *Overlay) openDiary() {
	now := time.Now()
	today := dayKey(now)
	o.diary, o.diaryPage = buildDiary(&o.cfg, now), 0
	if o.cfg.LastDiary != today {
		o.cfg.LastDiary = today
		o.cfg.Coins += 10 // propina por leerlo
		o.tipToday = true
		saveCfg(o.cfg)
	}
	o.scene = SceneDiary
}

// checkDiary: a partir de las 9 de la mañana, el diario del día se abre solo (una vez).
func (o *Overlay) checkDiary() {
	if o.tick%50 != 0 || time.Since(o.startedAt) < 8*time.Second || os.Getenv("MJ_DEMO") != "" {
		return
	}
	now := time.Now()
	if now.Hour() < 9 || o.cfg.LastDiary == dayKey(now) || o.scene != SceneNone || o.match != nil || o.nw != nil ||
		o.pet.Busy() || o.pet.drag || !o.byeAt.IsZero() {
		return
	}
	logf("diario: edicion de hoy")
	o.openDiary()
}

// worldNow: el mundo hasta ayer (se calcula una vez por día).
func (o *Overlay) worldNow() *World {
	if d := dayKey(time.Now()); o.world == nil || o.worldDay != d {
		o.world, o.worldDay = o.cfg.World(time.Now()), d
	}
	return o.world
}

// ---------- dibujo ----------

func (o *Overlay) closeBtn(s *ebiten.Image, x, y float64) {
	b := o.button("cerrar", x, y, 100, 28)
	rect(s, b.x, b.y, b.w, b.h, colCard)
	frame(s, b.x, b.y, b.w, b.h, colLine)
	txt(s, "CERRAR", b.x+b.w/2, b.y+7, 16, colInk, 0)
}

func (o *Overlay) drawToast(s *ebiten.Image, x, y float64) bool {
	if o.toast == "" || time.Since(o.toastAt) > 2500*time.Millisecond {
		return false
	}
	c := colOk
	if o.toast[:3] == "TE " {
		c = colErr
	}
	txt(s, o.toast, x, y, 8, c, -1)
	return true
}

func (o *Overlay) drawStore(s *ebiten.Image) {
	w, h := 640.0, 364.0
	x, y := o.panel(s, w, h)
	txt(s, "TIENDA", x+16, y+12, 8, colDim, -1)
	txt(s, fmt.Sprintf("¤ %d", o.cfg.Coins), x+w-16, y+8, 16, colAcc, 1)
	tw := (w - 32) / float64(len(storeCats))
	for i, c := range storeCats {
		b := o.button(fmt.Sprintf("st:t:%d", i), x+16+float64(i)*tw, y+34, tw-4, 24)
		col := colDim
		if i == o.storeTab {
			rect(s, b.x, b.y, b.w, b.h, colCard)
			col = colAcc
		}
		txt(s, c.label, b.x+b.w/2, b.y+8, 8, col, 0)
	}
	cat := storeCats[o.storeTab]
	n := catCount(cat.key)
	o.storeSel = min(o.storeSel, n-1)
	sel := o.storeSel

	// vista previa con lo elegido puesto
	prev := o.cfg.Av
	*slot(&prev, cat.key) = uint8(sel)
	rect(s, x+16, y+70, 130, 150, colCard)
	drawAvatar(s, x+16+65-6*8, y+70+60, 8, prev, o.tick, false)
	txt(s, itemName(cat.key, sel), x+16+65, y+70+130, 8, colInk, 0)
	label, col, fill := "PUESTO", colDim, false
	switch {
	case int(*slot(&o.cfg.Av, cat.key)) == sel:
	case o.cfg.Owns(cat.key, sel):
		label, col, fill = "PONER", colBg, true
	case o.cfg.Coins >= price(cat.key, sel):
		label, col, fill = fmt.Sprintf("COMPRAR %d", price(cat.key, sel)), colBg, true
	default:
		label = fmt.Sprintf("CUESTA %d ¤", price(cat.key, sel))
	}
	b := o.button("st:buy", x+16, y+230, 130, 30)
	if fill {
		rect(s, b.x, b.y, b.w, b.h, colAcc)
	} else {
		frame(s, b.x, b.y, b.w, b.h, colLine)
	}
	txt(s, label, b.x+b.w/2, b.y+11, 8, col, 0)

	// la lista: tres columnas de 8
	for i := 0; i < n; i++ {
		cx, cy := x+160+float64(i/8)*154, y+70+float64(i%8)*28
		b := o.button(fmt.Sprintf("st:i:%d", i), cx, cy, 150, 26)
		nc := colDim
		if i == sel {
			rect(s, b.x, b.y, b.w, b.h, colCard)
			nc = colInk
		}
		if cat.key == "color" {
			rect(s, cx+6, cy+9, 8, 8, avColors[i].col)
		}
		txt(s, itemName(cat.key, i), cx+18, cy+9, 8, nc, -1)
		st, sc := fmt.Sprintf("%d", price(cat.key, i)), colAcc
		switch {
		case int(*slot(&o.cfg.Av, cat.key)) == i:
			st, sc = "PUESTO", colOk
		case o.cfg.Owns(cat.key, i):
			st, sc = "TUYO", colDim
		}
		txt(s, st, cx+144, cy+9, 8, sc, 1)
	}
	if !o.drawToast(s, x+160, y+h-56) {
		txt(s, "CADA VICTORIA DA ¤ (MAS SI EL RIVAL ES DIFICIL)", x+160, y+h-56, 8, colLine, -1)
	}
	o.closeBtn(s, x+w-110, y+h-38)
}

func (o *Overlay) drawBoard(s *ebiten.Image) {
	rows := o.cfg.Leaderboard(o.worldNow())
	if len(rows) > 10 {
		rows = rows[:10]
	}
	w := 440.0
	h := 76 + float64(len(rows))*26 + 50
	x, y := o.panel(s, w, h)
	txt(s, "RANKING DE VICTORIAS", x+16, y+12, 8, colDim, -1)
	txt(s, "PONG", x+w-110, y+40, 8, colDim, 1)
	txt(s, "TOTAL", x+w-20, y+40, 8, colDim, 1)
	for i, r := range rows {
		ry := y + 58 + float64(i)*26
		if r.Me {
			rect(s, x+8, ry-5, w-16, 24, colCard)
		}
		rc := colDim
		if i == 0 {
			rc = colAcc
		}
		txt(s, fmt.Sprintf("%d", i+1), x+30, ry, 16, rc, 1)
		drawAvatar(s, x+42, ry+2, 1.5, r.Av, o.tick+i*17, false)
		txt(s, r.Name, x+68, ry, 16, r.Av.Col(), -1)
		if r.CPU {
			txt(s, "CPU", x+68+textW(r.Name, 16)+8, ry+6, 8, colLine, -1)
		} else if r.Me {
			txt(s, "TU", x+68+textW(r.Name, 16)+8, ry+6, 8, colAcc, -1)
		}
		txt(s, fmt.Sprintf("%d", r.Wins["pong"]), x+w-110, ry, 16, colDim, 1)
		txt(s, fmt.Sprintf("%d", r.Total), x+w-20, ry, 16, colInk, 1)
	}
	txt(s, "LOS CPU TAMBIEN JUEGAN", x+16, y+h-38, 8, colLine, -1)
	txt(s, "CUANDO NO ESTAS", x+16, y+h-24, 8, colLine, -1)
	o.closeBtn(s, x+w-110, y+h-38)
}

func clampInt(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}

func (o *Overlay) drawCPUPick(s *ebiten.Image) {
	w, h := 440.0, 46+float64(len(cpus))*46+12
	x, y := o.panel(s, w, h)
	txt(s, "ELIGE RIVAL", x+16, y+12, 8, colDim, -1)
	rec := map[string][2]int{}
	for _, m := range o.cfg.History {
		r := rec[m.Opp]
		if m.Won {
			r[0]++
		} else {
			r[1]++
		}
		rec[m.Opp] = r
	}
	for i, c := range cpus {
		ry := y + 34 + float64(i)*46
		b := o.button(fmt.Sprintf("cp:%d", i), x+6, ry, w-12, 44)
		if b.has(o.mx, o.my) {
			o.sel = i
		}
		if i == o.sel {
			rect(s, b.x, b.y, b.w, b.h, colCard)
		}
		drawAvatar(s, x+16, ry+12, 2.5, c.Av, o.tick+i*23, false)
		txt(s, c.Name, x+60, ry+6, 16, c.Av.Col(), -1)
		stars := ""
		for k := 0; k < 5; k++ {
			stars += map[bool]string{true: "★", false: "☆"}[k < c.Stars]
		}
		txt(s, stars, x+w-16, ry+8, 8, colAcc, 1)
		txt(s, c.Bio, x+60, ry+28, 8, colDim, -1)
		if r, ok := rec[c.ID]; ok {
			txt(s, fmt.Sprintf("%d-%d", r[0], r[1]), x+w-16, ry+28, 8, colInk, 1)
		}
	}
}

// ---------- Pong de 4 ----------

func (o *Overlay) inGame() bool { return o.scene == SceneMatch || o.scene == SceneMatch4 }

func (o *Overlay) updateMatch4(game bool) {
	m := o.m4
	if o.demo4 && m.Host() && !m.Started {
		for m.Players() < 4 {
			m.AddCPU()
		}
		m.Start()
	}
	if o.scene == SceneLobby4 && m.Started && m.MySide >= 0 {
		o.scene = SceneMatch4
		o.recorded, o.coinMsg = false, ""
		o.setGame(true) // las flechas pasan a ser del juego
	}
	dir := 0.0
	if game && m.MySide >= 0 {
		neg, pos := KLeft, KRight
		if m.MySide == SLeft || m.MySide == SRight {
			neg, pos = KUp, KDown
		}
		if gin.Down(neg) {
			dir--
		}
		if gin.Down(pos) {
			dir++
		}
		if botMode && m.p.Ph == PhPlay { // pruebas: sigue la bola sola
			dir = clamp((m.p.along(m.MySide)-(m.p.Pad[m.MySide]+PL4/2))/8, -1, 1)
		}
	}
	m.Update(dir, !game)
	if m.p.Ph != PhOver && o.recorded {
		o.recorded, o.coinMsg = false, ""
	}
	if over, won, w := m.Result(); over {
		o.recorded = true
		rec := MatchRec{Game: "pong4", Won: won, Me: m.p.Lives[max(m.MySide, 0)], Opp: "grupo", OppName: "X4"}
		if !won {
			rec.OppName = w.Av.DisplayName()
			switch {
			case w.Kind == SlotCPU:
				rec.Opp, rec.OppName = cpus[w.CPU].ID, cpus[w.CPU].Name
			case w.ID != "":
				rec.Opp = w.ID
			default:
				rec.Opp = "amigo"
			}
		}
		o.coinMsg = fmt.Sprintf("+%d ¤", o.cfg.Record(rec))
		saveCfg(o.cfg)
		logf("x4: %s (%s)", map[bool]string{true: "gane", false: "perdi"}[won], o.coinMsg)
	}
}
