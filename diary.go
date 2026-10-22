package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// El Diario Idle: todos los días a las 9 cuenta, con mucho drama, lo que pasó ayer en el mundo
// de los CPU y lo que hicieron los jugadores. (Sin tildes: la letra pixelada no las tiene bien.)

type dline struct {
	s    string
	c    color.NRGBA
	size float64
}

type Diary struct {
	Day   string
	No    int
	Date  string
	Pages [][]dline
}

const diaryCols, diaryRows = 56, 17 // caracteres por línea (letra de 8) y líneas por página

var (
	diasES  = []string{"DOMINGO", "LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES", "SABADO"}
	mesesES = []string{"ENE", "FEB", "MAR", "ABR", "MAY", "JUN", "JUL", "AGO", "SEP", "OCT", "NOV", "DIC"}
)

// Lo que dice cada CPU cuando gana o pierde ({B} es el rival).
var cpuWinLines = map[string][]string{
	"cpu:turbo": {"TURBO DIO UNA VUELTA OLIMPICA. SOLO. NADIE APLAUDIO.",
		"TURBO DICE QUE GANO SIN CALENTAR. LE CREEMOS A MEDIAS.",
		"TURBO PIDIO QUE LE HICIERAN UNA ESTATUA. LA RESPUESTA FUE NO."},
	"cpu:bigotes": {"BIGOTES CELEBRO CON UN TE, CON EL DEDITO ARRIBA.",
		"BIGOTES MIRO A {B} DE ARRIBA A ABAJO Y SE FUE SIN DECIR NADA.",
		"BIGOTES DECLARO: ERA OBVIO. SIEMPRE ES OBVIO."},
	"cpu:karl": {"KARL GRITO JUSTICIA Y NADIE SUPO POR QUE.",
		"KARL BAILO DE LADO DURANTE 40 MINUTOS.",
		"KARL TACHO A {B} DE SU LIBRETA DE VENGANZAS. QUEDAN 312."},
	"cpu:bit": {"BIT GANO Y DESAPARECIO ANTES DE LA FOTO.",
		"NADIE VIO A BIT GANAR. SOLO QUEDO EL MARCADOR.",
		"BIT DIJO YUPI MUY BAJITO. CASI NADIE LO ESCUCHO."},
	"cpu:mochi": {"MOCHI GANO DORMIDO. DE NUEVO. LA CIENCIA NO LO EXPLICA.",
		"MOCHI DESPERTO, VIO QUE HABIA GANADO Y SE VOLVIO A DORMIR.",
		"MOCHI NO SABE COMO GANO. {B} TAMPOCO."},
}

var cpuLoseLines = map[string][]string{
	"cpu:turbo": {"TURBO CULPO AL LAG. JUGABAN EN EL MISMO PC.",
		"TURBO PIDIO REVANCHA 14 VECES. SE LE DIJO QUE NO 14 VECES.",
		"TURBO DICE QUE SE DEJO GANAR. NADIE LE CREE."},
	"cpu:bigotes": {"BIGOTES TIRO LA PALETA AL SUELO. POR ACCIDENTE, DICE.",
		"BIGOTES EXIGIO HABLAR CON EL GERENTE DEL PONG.",
		"BIGOTES SE FUE OFENDIDO A LAMERSE UNA PATA."},
	"cpu:karl": {"KARL ANOTO A {B} EN SU LIBRETA DE VENGANZAS.",
		"KARL SE ENCERRO EN SU CAPARAZON Y NO QUIERE SALIR.",
		"KARL JURO VENGANZA. ES SU TERCERA VENGANZA DE LA SEMANA."},
	"cpu:bit": {"BIT SE VOLVIO INVISIBLE DE PURA PENA.",
		"BIT DIJO BUENO MUY BAJITO Y SE FUE FLOTANDO.",
		"BIT SE ESCONDIO DETRAS DE LA IMPRESORA HASTA LA TARDE."},
	"cpu:mochi": {"MOCHI NI SE ENTERO DE QUE PERDIO.",
		"MOCHI PERDIO, PERO SE COMIO LA PELOTA. EMPATE MORAL.",
		"MOCHI BOSTEZO DURANTE TODO EL PARTIDO. LITERAL."},
}

var cpuLifeLines = map[string][]string{
	"cpu:turbo": {"TURBO QUISO BATIR EL RECORD DE VELOCIDAD DEL PASILLO. CHOCO CON LA IMPRESORA.",
		"TURBO SE ACTUALIZO A TURBO 2.0. NADIE NOTO LA DIFERENCIA.",
		"TURBO ABRIO UN CURSO ONLINE DE PONG. TIENE 0 ALUMNOS.",
		"TURBO SE PUSO LENTES DE SOL ADENTRO DE LA OFICINA. TODO EL DIA."},
	"cpu:bigotes": {"BIGOTES TIRO UN VASO DE LA MESA MIRANDO A TODOS A LOS OJOS.",
		"BIGOTES DURMIO 16 HORAS Y LO LLAMO AUTOCUIDADO.",
		"BIGOTES ABRIO UN CLUB EXCLUSIVO. EL UNICO SOCIO ES BIGOTES.",
		"BIGOTES SE SENTO EN EL TECLADO JUSTO CUANDO ALGUIEN LO NECESITABA."},
	"cpu:karl": {"KARL ESCRIBIO UN POEMA DE 9 PAGINAS SOBRE UNA DERROTA DE 2019.",
		"KARL SE PELEO CON SU REFLEJO. PERDIO.",
		"KARL CAMINO DE LADO HASTA LA OFICINA. LLEGO TARDE, PERO CON ESTILO.",
		"KARL ORGANIZO UNA PROTESTA CONTRA LOS LUNES. FUE SOLO KARL."},
	"cpu:bit": {"BIT APARECIO EN LA FOTO DEL EQUIPO. NADIE SABE COMO.",
		"BIT ASUSTO AL CONSERJE SIN QUERER. PIDIO PERDON POR CORREO.",
		"BIT PASO EL DIA ADENTRO DE UNA PLANILLA EXCEL. CELDA B12.",
		"BIT DICE QUE VIO UN FANTASMA. LE RECORDAMOS QUE BIT ES UN FANTASMA."},
	"cpu:mochi": {"MOCHI DURMIO UNA SIESTA DE 6 HORAS ENCIMA DE UN TECLADO AJENO.",
		"MOCHI SE COMIO EL ALMUERZO DE TURBO. ESTABA AHI, DECLARO.",
		"MOCHI RODO POR LA ESCALERA. DICE QUE FUE A PROPOSITO.",
		"MOCHI SONO QUE GANABA EL MUNDIAL DE PONG. DESPERTO FELIZ."},
}

var duoLines = map[string][]string{
	"amistad": {"{A} Y {B} COMPARTIERON UNAS PAPAS FRITAS. FUE MUY EMOTIVO.",
		"{A} AYUDO A {B} A BUSCAR SU PALETA. ESTABA EN SU MANO.",
		"{A} Y {B} VIERON EL ATARDECER EN EL FONDO DE PANTALLA.",
		"{A} LE PRESTO SU CARGADOR A {B}. ESO ES AMOR VERDADERO."},
	"broma": {"{A} LE ESCONDIO LA PALETA A {B}. {B} NO SE RIO.",
		"{A} CAMBIO EL FONDO DE PANTALLA DE {B} POR UNA FOTO DE {A}.",
		"{A} LE PUSO UN COJIN PEDORRO A {B}. FUNCIONO PERFECTO.",
		"{A} LE DIO VUELTA EL MOUSE A {B}. {B} ESTUVO 2 HORAS CONFUNDIDO."},
	"pelea": {"{A} Y {B} DISCUTIERON 3 HORAS SOBRE SI LA PELOTA ES CUADRADA. LO ES.",
		"{A} LE DIJO A {B} QUE JUEGA COMO TOSTADORA. LA TOSTADORA SE OFENDIO.",
		"{A} Y {B} SE DEJARON DE SEGUIR EN TODAS LAS REDES.",
		"{A} Y {B} SE PELEARON POR LA ULTIMA GALLETA. GANO MOCHI, QUE PASABA POR AHI."},
}

var visitLines = map[string]string{
	"cpu:turbo":   "TURBO RETO A {P} A UNA CARRERA. {P} NO ESTABA. TURBO SE DECLARO GANADOR.",
	"cpu:bigotes": "BIGOTES INSPECCIONO EL ESCRITORIO DE {P}. NOTA FINAL: 4 DE 10.",
	"cpu:karl":    "KARL ESPERO A {P} TODA LA NOCHE PARA UNA REVANCHA. SE DURMIO A LAS 9:05.",
	"cpu:bit":     "BIT ESTUVO EN EL ESCRITORIO DE {P}. O ESO CREEMOS.",
	"cpu:mochi":   "MOCHI SE DURMIO EN EL TECLADO DE {P}. ESCRIBIO 4000 LETRAS Z.",
}

var visitAny = []string{
	"{A} PASO POR EL ESCRITORIO DE {P} Y MOVIO TODO 2 CM A LA IZQUIERDA.",
	"{A} LE DEJO UNA NOTA A {P}: TE ESPERO EN LA CANCHA.",
	"{A} SE SENTO EN LA SILLA DE {P} Y DIJO: AHORA YO SOY {P}.",
	"{A} LE DEJO UN REGALO A {P}: UN CALCETIN. SOLO UNO.",
}

func diaryDate(t time.Time) string {
	return fmt.Sprintf("%s %d %s", diasES[t.Weekday()], t.Day(), mesesES[t.Month()-1])
}

// wrapText corta un texto en líneas de n caracteres como mucho.
func wrapText(s string, n int) []string {
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		if line != "" && len([]rune(line))+1+len([]rune(w)) > n {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// worldPlayers: los nombres de los jugadores reales que conoce este PC (yo y mis amigos).
func (c *Config) worldPlayers() []string {
	names := []string{c.Av.DisplayName()}
	for _, f := range c.Friends {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

// World: el mundo hasta ayer (lo de hoy pasa esta noche).
func (c *Config) World(now time.Time) *World {
	return simulate(c.Room, now.AddDate(0, 0, -1), c.worldPlayers(), c.History)
}

func buildDiary(c *Config, now time.Time) *Diary {
	today := dayKey(now)
	yday := now.AddDate(0, 0, -1)
	yk := dayKey(yday)
	me := c.Av.DisplayName()
	w := simulate(c.Room, yday, c.worldPlayers(), c.History)
	r := rand.New(rand.NewSource(seedOf(c.Room, today, "diario")))
	used := map[string]bool{}
	choose := func(l []string, start int) string { // no repite la misma frase en el mismo diario
		for k := 0; k < len(l); k++ {
			if x := l[(start+k)%len(l)]; !used[x] {
				used[x] = true
				return x
			}
		}
		return l[start%len(l)]
	}
	pick := func(l []string) string { return choose(l, r.Intn(len(l))) }
	name := func(id string) string {
		if id == "yo" {
			return me
		}
		return nameOf(id)
	}
	fill := func(s string, kv ...string) string { return strings.NewReplacer(kv...).Replace(s) }

	d := &Diary{Day: today, No: int(now.Sub(worldStart).Hours()/24) + 1, Date: diaryDate(now)}
	if d.No < 1 {
		d.No = 1
	}
	var paras []dline
	add := func(s string, col color.NRGBA) { paras = append(paras, dline{s: s, c: col, size: 8}) }
	gap := func() { paras = append(paras, dline{}) }

	if c.LastDiary == "" { // primera edición: presentación
		add("¡BIENVENIDOS AL DIARIO IDLE!", colAcc)
		add("TODOS LOS DIAS A LAS 9 TE CONTAMOS LO QUE PASO MIENTRAS TRABAJABAS. CONOCE A LOS VECINOS:", colInk)
		for _, cp := range cpus {
			add(cp.Name+": "+cp.Bio+".", cp.Av.Col())
		}
		add("JUEGAN SOLOS CUANDO NO ESTAS, SE HACEN AMIGOS, SE ODIAN Y SUBEN EN EL RANKING. NO DEJES QUE TE PASEN.", colDim)
		gap()
	}

	evs := w.Days[yk]
	// titular: lo más jugoso del día
	head := "UN DIA TRANQUILO EN IDLE. (MENTIRA.)"
	best := 0
	for _, e := range evs {
		score, h := 0, ""
		switch e.Kind {
		case "enemies":
			score, h = 6, fill("¡ESCANDALO! {A} Y {B} ROMPEN RELACIONES", "{A}", name(e.A), "{B}", name(e.B))
		case "friends":
			score, h = 5, fill("¡NACE UNA AMISTAD! {A} Y {B} INSEPARABLES", "{A}", name(e.A), "{B}", name(e.B))
		case "peace":
			score, h = 4, fill("¡PAZ MUNDIAL! {A} Y {B} SE ABRAZAN", "{A}", name(e.A), "{B}", name(e.B))
		case "match":
			wa, lb := cpuByID(e.A), cpuByID(e.B)
			switch {
			case lb.Skill-wa.Skill > .15:
				score, h = 4, fill("¡SORPRESA! {A} DERROTA A {B}", "{A}", wa.Name, "{B}", lb.Name)
			case e.N >= 3:
				score, h = 3, fill("{A} ESTA IMPARABLE: {N} SEGUIDAS", "{A}", wa.Name, "{N}", fmt.Sprint(e.N))
			case e.SB == 0:
				score, h = 3, fill("PALIZA: {A} 7-0 {B}", "{A}", wa.Name, "{B}", lb.Name)
			}
		case "real":
			if e.Tag == "gane" && cpuByID(e.A) != nil {
				score, h = 2, fill("{P} HUMILLA A {A}", "{P}", me, "{A}", nameOf(e.A))
			}
		}
		if score > best {
			best, head = score, h
		}
	}
	paras = append(paras, dline{s: head, c: colAcc, size: 16})
	gap()

	if len(evs) == 0 && c.LastDiary != "" {
		add("AYER NO PASO NADA. LOS CPU ESTABAN DE VACACIONES. SOSPECHOSO.", colDim)
	}
	// partidas de los CPU entre ellos
	for _, e := range evs {
		if e.Kind != "match" {
			continue
		}
		wa, lb := cpuByID(e.A), cpuByID(e.B)
		s := fill("{A} LE GANO {S} A {B}.", "{A}", wa.Name, "{B}", lb.Name, "{S}", fmt.Sprintf("%d-%d", e.SA, e.SB))
		switch {
		case e.SB == 0:
			s += " SIN PIEDAD."
		case e.SB == 6:
			s += " AL BORDE DEL INFARTO."
		}
		if r.Intn(2) == 0 {
			s += " " + fill(pick(cpuWinLines[wa.ID]), "{B}", lb.Name)
		} else {
			s += " " + fill(pick(cpuLoseLines[lb.ID]), "{B}", wa.Name)
		}
		add(s, wa.Av.Col())
	}
	// vida diaria, parejas y relaciones
	for _, e := range evs {
		switch e.Kind {
		case "life":
			add(choose(cpuLifeLines[e.A], e.N), colInk)
		case "duo":
			add(fill(choose(duoLines[e.Tag], e.N), "{A}", name(e.A), "{B}", name(e.B)), colInk)
		case "friends":
			add(fill("¡NUEVA AMISTAD! {A} Y {B} AHORA SON INSEPARABLES. YA TIENEN PULSERAS IGUALES.", "{A}", name(e.A), "{B}", name(e.B)), colOk)
		case "enemies":
			add(fill("¡ENEMISTAD OFICIAL! {A} Y {B} YA NO SE HABLAN. SOLO SE MIRAN FEO.", "{A}", name(e.A), "{B}", name(e.B)), colErr)
		case "peace":
			add(fill("{A} Y {B} HICIERON LAS PACES. TODA LA OFICINA LLORO.", "{A}", name(e.A), "{B}", name(e.B)), colOk)
		case "visit":
			s := visitLines[e.A]
			if e.N%2 == 0 {
				s = visitAny[e.N/2%len(visitAny)]
			}
			add(fill(s, "{A}", nameOf(e.A), "{P}", e.B), colDim)
		}
	}

	// lo que hicieron los jugadores
	type tally struct {
		name      string
		won, lost int
	}
	var order []string
	tl := map[string]*tally{}
	for _, m := range c.History {
		if m.Day != yk {
			continue
		}
		t := tl[m.Opp]
		if t == nil {
			n := m.OppName
			if cp := cpuByID(m.Opp); cp != nil {
				n = cp.Name
			}
			t = &tally{name: n}
			tl[m.Opp] = t
			order = append(order, m.Opp)
		}
		if m.Won {
			t.won++
		} else {
			t.lost++
		}
	}
	if len(order) > 0 {
		gap()
	}
	for _, id := range order {
		t := tl[id]
		s := fill("{P} JUGO {N} CONTRA {B}: GANO {W} Y PERDIO {L}.", "{P}", me, "{N}", plural(t.won+t.lost, "PARTIDA", "PARTIDAS"),
			"{B}", t.name, "{W}", fmt.Sprint(t.won), "{L}", fmt.Sprint(t.lost))
		if cp := cpuByID(id); cp != nil {
			if t.won > t.lost {
				s += " " + fill(pick(cpuLoseLines[id]), "{B}", me)
			} else {
				s += " " + fill(pick(cpuWinLines[id]), "{B}", me)
			}
		} else if t.won > t.lost {
			s += " " + fill("SE RUMOREA QUE {B} PIDIO LA REVANCHA DE RODILLAS.", "{B}", t.name)
		} else if t.lost > t.won {
			s += " " + fill("{B} LO CUENTA EN CADA REUNION.", "{B}", t.name)
		} else {
			s += " EMPATE TECNICO. LA TENSION SE CORTA CON CUCHILLO."
		}
		add(s, c.Av.Col())
	}
	for _, b := range c.Bought {
		if b.Day == yk {
			add(fill(pick([]string{"{P} ESTRENO {X}. BIGOTES DICE QUE ES MUCHO.", "{P} SE COMPRO {X}. TURBO DICE QUE SE LO COPIO.",
				"{P} APARECIO CON {X}. KARL LO CONSIDERA UNA PROVOCACION."}), "{P}", me, "{X}", b.Item), colAcc)
		}
	}
	if len(order) == 0 && c.LastDiary != "" {
		add(fill(pick([]string{"{P} NO JUGO NADA AYER. LOS CPU EMPIEZAN A TRAMAR COSAS.",
			"NI UNA PARTIDA DE {P} AYER. KARL LO TOMA COMO ALGO PERSONAL.",
			"{P} SE TOMO EL DIA LIBRE DEL PONG. TURBO YA SE CREE EL DUENO DE LA CANCHA."}), "{P}", me), colDim)
	}

	// ranking
	board := c.leaderboardAt(w, yk)
	gap()
	if len(board) > 0 {
		l := board[0]
		s := fmt.Sprintf("RANKING: LIDERA %s CON %s.", l.Name, plural(l.Total, "VICTORIA", "VICTORIAS"))
		if l.CPU {
			s += " ¡LOS HUMANOS DEBEN REACCIONAR!"
		}
		add(s, colInk)
		prev := c.leaderboardAt(simulate(c.Room, yday.AddDate(0, 0, -1), c.worldPlayers(), c.History), dayKey(yday.AddDate(0, 0, -1)))
		pos := map[string]int{}
		for i, b := range prev {
			pos[b.ID] = i
		}
		for i, b := range board {
			if p, ok := pos[b.ID]; ok && i < p && b.CPU {
				add(fill("{A} SUBIO AL PUESTO #{N}. YA PIDE AUTOGRAFOS.", "{A}", b.Name, "{N}", fmt.Sprint(i+1)), b.Av.Col())
				break
			}
		}
	}

	// a líneas y páginas
	var lines []dline
	for _, p := range paras {
		if p.s == "" {
			lines = append(lines, p)
			continue
		}
		n := diaryCols
		if p.size > 8 {
			n = diaryCols / 2
		}
		for _, l := range wrapText(p.s, n) {
			lines = append(lines, dline{s: l, c: p.c, size: p.size})
		}
	}
	var page []dline
	nrow := 0
	for _, l := range lines {
		h := 1
		if l.size > 8 {
			h = 2
		}
		if l.s == "" && nrow == 0 {
			continue // la página no empieza con un espacio
		}
		if nrow+h > diaryRows {
			d.Pages = append(d.Pages, page)
			page, nrow = nil, 0
			if l.s == "" {
				continue
			}
		}
		page = append(page, l)
		nrow += h
	}
	if len(page) > 0 {
		d.Pages = append(d.Pages, page)
	}
	return d
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
