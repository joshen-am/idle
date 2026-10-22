package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// El Diario Idle: todos los días a las 9 cuenta, con mucho drama, lo que pasó ayer en el mundo
// de los CPU y lo que hicieron los jugadores. (Sin tildes: la letra pixelada no las tiene bien.)

// Una edición: varias páginas de diario antiguo, con "fotos" de los personajes.
type Photo struct {
	Kind string   // win, duo, prank, fight, peace, portrait, visit, podium, cast
	Avs  []Avatar // protagonistas
	Tag  string   // variante (portrait: sleep, smug, angry, fade, fast)
}

type Article struct {
	Title, Text string
	Photo       *Photo
}

type Result struct {
	WAv, LAv Avatar
	W, L     string
	SW, SL   int
}

type DPage struct {
	Section string
	Head    string // titular (portada)
	Arts    []Article
	Results []Result
	Ad      string
}

type Diary struct {
	Day   string
	No    int
	Date  string
	Pages []DPage
}


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
	avOf := func(id string) Avatar {
		if cp := cpuByID(id); cp != nil {
			return cp.Av
		}
		if id == "yo" {
			return c.Av
		}
		for _, f := range c.Friends {
			if f.ID == id || f.Name == id {
				return f.Av
			}
		}
		return avAny
	}
	portraitTag := map[string]string{"cpu:mochi": "sleep", "cpu:bigotes": "smug", "cpu:karl": "angry", "cpu:bit": "fade", "cpu:turbo": "fast"}

	evs := w.Days[yk]
	front := DPage{Section: "PORTADA", Head: "UN DIA TRANQUILO EN IDLE. (MENTIRA.)"}
	sports := DPage{Section: "DEPORTES"}
	social := DPage{Section: "SOCIEDAD"}
	hood := DPage{Section: "VECINOS"}

	// portada: lo más jugoso del día
	best := 0
	for _, e := range evs {
		score, h, txt := 0, "", ""
		var ph *Photo
		switch e.Kind {
		case "enemies":
			score, h = 6, fill("¡ESCANDALO! {A} Y {B} ROMPEN RELACIONES", "{A}", name(e.A), "{B}", name(e.B))
			txt = "YA NO SE HABLAN. SOLO SE MIRAN FEO DESDE LEJOS. LA OFICINA ESTA DIVIDIDA."
			ph = &Photo{Kind: "fight", Avs: []Avatar{avOf(e.A), avOf(e.B)}}
		case "friends":
			score, h = 5, fill("¡NACE UNA AMISTAD! {A} Y {B} INSEPARABLES", "{A}", name(e.A), "{B}", name(e.B))
			txt = "YA TIENEN PULSERAS IGUALES Y UN SALUDO SECRETO QUE NADIE ENTIENDE."
			ph = &Photo{Kind: "duo", Avs: []Avatar{avOf(e.A), avOf(e.B)}}
		case "peace":
			score, h = 4, fill("¡PAZ MUNDIAL! {A} Y {B} SE ABRAZAN", "{A}", name(e.A), "{B}", name(e.B))
			txt = "TRAS SEMANAS DE MIRADAS FEAS, HICIERON LAS PACES. TODA LA OFICINA LLORO."
			ph = &Photo{Kind: "peace", Avs: []Avatar{avOf(e.A), avOf(e.B)}}
		case "match":
			wa, lb := cpuByID(e.A), cpuByID(e.B)
			ph = &Photo{Kind: "win", Avs: []Avatar{wa.Av, lb.Av}}
			txt = fill("{A} LE GANO {S} A {B}. ", "{A}", wa.Name, "{B}", lb.Name, "{S}", fmt.Sprintf("%d-%d", e.SA, e.SB)) +
				fill(pick(cpuLoseLines[lb.ID]), "{B}", wa.Name)
			switch {
			case lb.Skill-wa.Skill > .15:
				score, h = 4, fill("¡SORPRESA! {A} DERROTA A {B}", "{A}", wa.Name, "{B}", lb.Name)
			case e.N >= 3:
				score, h = 3, fill("{A} ESTA IMPARABLE: {N} SEGUIDAS", "{A}", wa.Name, "{N}", fmt.Sprint(e.N))
			case e.SB == 0:
				score, h = 3, fill("PALIZA: {A} 7-0 {B}", "{A}", wa.Name, "{B}", lb.Name)
			default:
				score, h = 1, fill("{A} SE IMPONE A {B}", "{A}", wa.Name, "{B}", lb.Name)
			}
		case "real":
			if e.Tag == "gane" && cpuByID(e.A) != nil {
				score, h = 2, fill("{P} HUMILLA A {A}", "{P}", me, "{A}", nameOf(e.A))
				ph = &Photo{Kind: "win", Avs: []Avatar{c.Av, avOf(e.A)}}
				txt = fill("{P} LE GANO {S} A {A}. ", "{P}", me, "{A}", nameOf(e.A), "{S}", fmt.Sprintf("%d-%d", e.SA, e.SB)) +
					fill(pick(cpuLoseLines[e.A]), "{B}", me)
			}
		}
		if score > best {
			best, front.Head = score, h
			front.Arts = []Article{{Text: txt, Photo: ph}}
		}
	}
	if len(front.Arts) == 0 {
		cp := cpus[r.Intn(len(cpus))]
		front.Arts = []Article{{Text: "LOS CPU SE PASARON EL DIA MIRANDO EL TECHO. ALGO TRAMAN.",
			Photo: &Photo{Kind: "portrait", Avs: []Avatar{cp.Av}, Tag: portraitTag[cp.ID]}}}
	}
	if c.LastDiary == "" { // primera edición
		front.Head = "NACE EL DIARIO IDLE"
		front.Arts = []Article{{Text: "CADA DIA A LAS 9 LES CONTAREMOS LO QUE PASO MIENTRAS TRABAJABAN. " +
			"CONOZCAN A LOS VECINOS EN LA PAGINA 2.", Photo: &Photo{Kind: "cast"}}}
		cast := DPage{Section: "LOS VECINOS"}
		for _, cp := range cpus {
			cast.Arts = append(cast.Arts, Article{Title: cp.Name, Text: cp.Bio + ".",
				Photo: &Photo{Kind: "portrait", Avs: []Avatar{cp.Av}, Tag: portraitTag[cp.ID]}})
		}
		d.Pages = append(d.Pages, front, cast)
		front.Section = "" // ya se agregó
	}

	// deportes: resultados de los CPU y de los jugadores
	var quote *Article
	for _, e := range evs {
		if e.Kind != "match" {
			continue
		}
		wa, lb := cpuByID(e.A), cpuByID(e.B)
		sports.Results = append(sports.Results, Result{WAv: wa.Av, LAv: lb.Av, W: wa.Name, L: lb.Name, SW: e.SA, SL: e.SB})
		if quote == nil {
			lines, who := cpuWinLines[wa.ID], wa
			if r.Intn(2) == 0 {
				lines, who = cpuLoseLines[lb.ID], lb
			}
			q := fill(pick(lines), "{B}", map[bool]string{true: lb.Name, false: wa.Name}[who == wa])
			quote = &Article{Title: "LA FRASE DEL DIA", Text: q, Photo: &Photo{Kind: "portrait", Avs: []Avatar{who.Av}, Tag: portraitTag[who.ID]}}
		}
	}
	for _, m := range c.History {
		if m.Day != yk {
			continue
		}
		oa := avOf(m.Opp)
		if m.OppName == "" {
			m.OppName = nameOf(m.Opp)
		}
		if m.Won {
			sports.Results = append(sports.Results, Result{WAv: c.Av, LAv: oa, W: me, L: m.OppName, SW: m.Me, SL: m.Them})
		} else {
			sports.Results = append(sports.Results, Result{WAv: oa, LAv: c.Av, W: m.OppName, L: me, SW: m.Them, SL: m.Me})
		}
	}
	if len(sports.Results) > 7 { // resumido: lo último
		sports.Results = sports.Results[len(sports.Results)-7:]
	}
	if quote != nil {
		sports.Arts = append(sports.Arts, *quote)
	}

	// sociedad: amistades, bromas y peleas
	for _, e := range evs {
		var a Article
		A, B := avOf(e.A), avOf(e.B)
		switch e.Kind {
		case "duo":
			kind := map[string]string{"amistad": "duo", "broma": "prank", "pelea": "fight"}[e.Tag]
			a = Article{Title: map[string]string{"amistad": "TERNURA", "broma": "BROMA", "pelea": "PELEA"}[e.Tag],
				Text: fill(choose(duoLines[e.Tag], e.N), "{A}", name(e.A), "{B}", name(e.B)), Photo: &Photo{Kind: kind, Avs: []Avatar{A, B}}}
		case "friends":
			a = Article{Title: "NUEVA AMISTAD", Text: fill("{A} Y {B} SON OFICIALMENTE MEJORES AMIGOS.", "{A}", name(e.A), "{B}", name(e.B)),
				Photo: &Photo{Kind: "duo", Avs: []Avatar{A, B}}}
		case "enemies":
			a = Article{Title: "ENEMISTAD", Text: fill("{A} Y {B} YA NO SE HABLAN.", "{A}", name(e.A), "{B}", name(e.B)),
				Photo: &Photo{Kind: "fight", Avs: []Avatar{A, B}}}
		case "peace":
			a = Article{Title: "PACES", Text: fill("{A} Y {B} HICIERON LAS PACES.", "{A}", name(e.A), "{B}", name(e.B)),
				Photo: &Photo{Kind: "peace", Avs: []Avatar{A, B}}}
		default:
			continue
		}
		if len(social.Arts) < 3 {
			social.Arts = append(social.Arts, a)
		}
	}

	// vecinos: la vida diaria, visitas y lo que se compraron
	for _, e := range evs {
		switch e.Kind {
		case "life":
			if len(hood.Arts) < 2 {
				hood.Arts = append(hood.Arts, Article{Title: nameOf(e.A), Text: choose(cpuLifeLines[e.A], e.N),
					Photo: &Photo{Kind: "portrait", Avs: []Avatar{avOf(e.A)}, Tag: portraitTag[e.A]}})
			}
		case "visit":
			s := visitLines[e.A]
			if e.N%2 == 0 {
				s = visitAny[e.N/2%len(visitAny)]
			}
			hood.Arts = append(hood.Arts, Article{Title: "VISITA", Text: fill(s, "{A}", nameOf(e.A), "{P}", e.B),
				Photo: &Photo{Kind: "visit", Avs: []Avatar{avOf(e.A)}}})
		}
	}
	for _, b := range c.Bought {
		if b.Day == yk {
			hood.Arts = append(hood.Arts, Article{Title: "MODA", Text: fill(pick([]string{"{P} ESTRENO {X}. BIGOTES DICE QUE ES MUCHO.",
				"{P} SE COMPRO {X}. TURBO DICE QUE SE LO COPIO.", "{P} APARECIO CON {X}. KARL LO CONSIDERA UNA PROVOCACION."}), "{P}", me, "{X}", b.Item),
				Photo: &Photo{Kind: "portrait", Avs: []Avatar{c.Av}, Tag: "smug"}})
			break
		}
	}
	played := false
	for _, m := range c.History {
		played = played || m.Day == yk
	}
	if !played && c.LastDiary != "" {
		hood.Arts = append(hood.Arts, Article{Title: "AUSENTE", Text: fill(pick([]string{"{P} NO JUGO NADA AYER. LOS CPU EMPIEZAN A TRAMAR COSAS.",
			"NI UNA PARTIDA DE {P} AYER. KARL LO TOMA COMO ALGO PERSONAL.",
			"{P} SE TOMO EL DIA LIBRE. TURBO YA SE CREE EL DUENO DE LA CANCHA."}), "{P}", me),
			Photo: &Photo{Kind: "portrait", Avs: []Avatar{c.Av}, Tag: "sleep"}})
	}
	if len(hood.Arts) > 4 {
		hood.Arts = hood.Arts[:4]
	}

	// ranking y clasificados
	board := c.leaderboardAt(w, yk)
	rank := DPage{Section: "RANKING", Ad: pick(classifieds) + "\n" + pick(classifieds)}
	var top []Avatar
	for i := 0; i < len(board) && i < 3; i++ {
		top = append(top, board[i].Av)
	}
	text := ""
	if len(board) > 0 {
		l := board[0]
		text = fmt.Sprintf("LIDERA %s CON %s.", l.Name, plural(l.Total, "VICTORIA", "VICTORIAS"))
		if l.CPU {
			text += " ¡LOS HUMANOS DEBEN REACCIONAR!"
		}
		for i, b := range board {
			if b.Me && i > 0 {
				text += fmt.Sprintf(" {P} VA #%d.", i+1)
			}
		}
		prev := c.leaderboardAt(simulate(c.Room, yday.AddDate(0, 0, -1), c.worldPlayers(), c.History), dayKey(yday.AddDate(0, 0, -1)))
		pos := map[string]int{}
		for i, b := range prev {
			pos[b.ID] = i
		}
		for i, b := range board {
			if p, ok := pos[b.ID]; ok && i < p && b.CPU {
				text += fill(" {A} SUBIO AL #{N} Y YA PIDE AUTOGRAFOS.", "{A}", b.Name, "{N}", fmt.Sprint(i+1))
				break
			}
		}
	}
	rank.Arts = []Article{{Text: fill(text, "{P}", me), Photo: &Photo{Kind: "podium", Avs: top}}}

	if n := len(social.Arts) + len(hood.Arts); n > 0 && n <= 4 { // poca noticia: una sola página
		social.Section = "SOCIEDAD Y VECINOS"
		social.Arts = append(social.Arts, hood.Arts...)
		hood.Arts = nil
	}
	for _, p := range []DPage{front, sports, social, hood, rank} {
		if p.Section != "" && (len(p.Arts) > 0 || len(p.Results) > 0) {
			d.Pages = append(d.Pages, p)
		}
	}
	return d
}

var classifieds = []string{
	"SE VENDE PALETA CASI NUEVA. SOLO PERDIO 40 PARTIDOS. PREGUNTAR POR TURBO.",
	"BUSCO SOCIO DE SIESTA. REQUISITO: NO RONCAR. ESCRIBIR A MOCHI.",
	"KARL DA CLASES DE RENCOR. LA PRIMERA ES GRATIS. LA SEGUNDA NO SE OLVIDA.",
	"SE BUSCA CALCETIN PERDIDO. RECOMPENSA: OTRO CALCETIN. BIT.",
	"BIGOTES ARRIENDA CAJA DE CARTON CON VISTA AL TECLADO. SOLO GENTE ELEGANTE.",
	"SE REGALA TROFEO DE PARTICIPACION. NUNCA USADO. TURBO.",
	"VENDO ANTENA DE ROBOT. CAPTA 3 CANALES Y UN CHISME.",
	"SE NECESITA ARBITRO PARA PONG. QUE NO LE TENGA MIEDO A KARL.",
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
