package main

import (
	"hash/fnv"
	"math/rand"
	"time"
)

// El mundo de Idle: 5 personajes CPU con personalidad que juegan entre ellos todos los días,
// se hacen amigos o enemigos y molestan a los jugadores cuando no están. Todo se calcula a
// partir de la fecha y el nombre de la sala, así que en los dos PCs pasa exactamente lo mismo
// sin necesidad de un servidor.

type CPUChar struct {
	ID, Name string
	Av       Avatar
	Skill    float64 // 0..1: qué tan bueno es (también en las partidas que juegan solos)
	Speed    float64 // en Pong: velocidad de la paleta (0..1)
	Err      float64 // en Pong: cuánto le erra (1 = normal)
	Stars    int
	Bio      string // personalidad, en una línea
}

var cpus = []CPUChar{
	{ID: "cpu:turbo", Name: "TURBO", Av: Avatar{Body: 4, Color: 3, Eyes: 5, Hat: 6, Name: "TURBO"},
		Skill: .72, Speed: .95, Err: 1.25, Stars: 4, Bio: "RAPIDO, TORPE Y MUY ARROGANTE"},
	{ID: "cpu:bigotes", Name: "BIGOTES", Av: Avatar{Body: 5, Color: 7, Eyes: 4, Hat: 2, Name: "BIGOTES"},
		Skill: .78, Speed: .85, Err: 1.12, Stars: 5, Bio: "ELEGANTE Y PESIMO PERDEDOR"},
	{ID: "cpu:karl", Name: "KARL", Av: Avatar{Body: 3, Color: 10, Eyes: 1, Hat: 8, Name: "KARL"},
		Skill: .55, Speed: .75, Err: 1.3, Stars: 3, Bio: "DRAMATICO Y RENCOROSO"},
	{ID: "cpu:bit", Name: "BIT", Av: Avatar{Body: 2, Color: 4, Eyes: 0, Hat: 0, Name: "BIT"},
		Skill: .5, Speed: .6, Err: 1.35, Stars: 3, Bio: "TIMIDO, APARECE Y DESAPARECE"},
	{ID: "cpu:mochi", Name: "MOCHI", Av: Avatar{Body: 1, Color: 5, Eyes: 3, Hat: 4, Name: "MOCHI"},
		Skill: .35, Speed: .55, Err: 1.45, Stars: 2, Bio: "DORMILON, PERO CON SUERTE"},
}

func cpuByID(id string) *CPUChar {
	for i := range cpus {
		if cpus[i].ID == id {
			return &cpus[i]
		}
	}
	return nil
}

func nameOf(id string) string {
	if c := cpuByID(id); c != nil {
		return c.Name
	}
	return id
}

// worldStart: desde este día los CPU juegan solos.
var worldStart = time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)

// WEvent: algo que pasó en el mundo (el diario lo cuenta con gracia).
type WEvent struct {
	Kind   string // match, life, duo, visit, friends, enemies, peace, real
	A, B   string // protagonistas (ids de CPU, o nombres de jugadores)
	SA, SB int    // marcador
	Tag    string // variante (amistad, broma, pelea...)
	N      int
}

type relKey [2]string

func rk(a, b string) relKey {
	if a > b {
		a, b = b, a
	}
	return relKey{a, b}
}

type World struct {
	Wins   map[string]int // victorias de los CPU jugando entre ellos
	Rel    map[relKey]int // -10 enemigos ... +10 mejores amigos
	Streak map[string]int // racha de victorias
	Days   map[string][]WEvent
}

func seedOf(parts ...string) int64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// relChange ajusta la relación y anota si cambió de estado (amigos, enemigos, paces).
func (w *World) relChange(day string, a, b string, d int) {
	k := rk(a, b)
	before := w.Rel[k]
	after := before + d
	if after > 10 {
		after = 10
	}
	if after < -10 {
		after = -10
	}
	w.Rel[k] = after
	switch {
	case before < 5 && after >= 5:
		w.Days[day] = append(w.Days[day], WEvent{Kind: "friends", A: a, B: b})
	case before > -5 && after <= -5:
		w.Days[day] = append(w.Days[day], WEvent{Kind: "enemies", A: a, B: b})
	case before <= -5 && after > -5:
		w.Days[day] = append(w.Days[day], WEvent{Kind: "peace", A: a, B: b})
	}
}

// simulate calcula el mundo desde worldStart hasta el día `upTo` (incluido). `players` son los
// nombres de los jugadores reales que este PC conoce; `hist` las partidas de este PC.
func simulate(room string, upTo time.Time, players []string, hist []MatchRec) *World {
	w := &World{Wins: map[string]int{}, Rel: map[relKey]int{}, Streak: map[string]int{}, Days: map[string][]WEvent{}}
	for d := worldStart; !d.After(upTo); d = d.AddDate(0, 0, 1) {
		day := dayKey(d)
		// lo de los CPU entre ellos sale igual en todos los PCs (no depende de los jugadores)
		r := rand.New(rand.NewSource(seedOf(room, day, "cpu")))
		for i, n := 0, 2+r.Intn(2); i < n; i++ {
			a, b := r.Intn(len(cpus)), r.Intn(len(cpus)-1)
			if b >= a {
				b++
			}
			ca, cb := cpus[a], cpus[b]
			pa := .5 + (ca.Skill-cb.Skill)*.9 // el mejor suele ganar, pero no siempre
			if ca.ID == "cpu:mochi" || cb.ID == "cpu:mochi" {
				pa = .5 + (pa-.5)*.5 // Mochi tiene una suerte inexplicable
			}
			win, lose := ca, cb
			if r.Float64() > pa {
				win, lose = cb, ca
			}
			ls := r.Intn(7)
			w.Wins[win.ID]++
			w.Streak[win.ID]++
			w.Streak[lose.ID] = 0
			w.Days[day] = append(w.Days[day], WEvent{Kind: "match", A: win.ID, B: lose.ID, SA: 7, SB: ls, N: w.Streak[win.ID]})
			d := -1
			if ls <= 1 {
				d = -2 // las palizas dejan rencor
			}
			if lose.ID == "cpu:karl" {
				d-- // Karl nunca olvida
			}
			w.relChange(day, win.ID, lose.ID, d)
		}
		for i, n := 0, 1+r.Intn(2); i < n; i++ {
			c := cpus[r.Intn(len(cpus))]
			w.Days[day] = append(w.Days[day], WEvent{Kind: "life", A: c.ID, N: r.Intn(100)})
		}
		if r.Float64() < .6 {
			a, b := r.Intn(len(cpus)), r.Intn(len(cpus)-1)
			if b >= a {
				b++
			}
			ca, cb := cpus[a].ID, cpus[b].ID
			tag, d := "amistad", 2
			switch rel := w.Rel[rk(ca, cb)]; {
			case rel <= -3 && r.Float64() < .6:
				tag, d = "pelea", -1
			case r.Float64() < .35:
				tag, d = "broma", -2
			}
			w.Days[day] = append(w.Days[day], WEvent{Kind: "duo", A: ca, B: cb, Tag: tag, N: r.Intn(100)})
			w.relChange(day, ca, cb, d)
		}

		// lo que tiene que ver con los jugadores reales (puede variar un poco entre PCs)
		rp := rand.New(rand.NewSource(seedOf(room, day, "jugadores")))
		if len(players) > 0 && rp.Float64() < .7 {
			c := cpus[rp.Intn(len(cpus))]
			w.Days[day] = append(w.Days[day], WEvent{Kind: "visit", A: c.ID, B: players[rp.Intn(len(players))], N: rp.Intn(100)})
		}
		for _, m := range hist {
			if m.Day != day {
				continue
			}
			w.Days[day] = append(w.Days[day], WEvent{Kind: "real", A: m.Opp, B: m.OppName, SA: m.Me, SB: m.Them, Tag: map[bool]string{true: "gane", false: "perdi"}[m.Won]})
			if cpuByID(m.Opp) != nil {
				if m.Won {
					w.relChange(day, m.Opp, "yo", -1)
				} else {
					w.relChange(day, m.Opp, "yo", 1) // ganarte le cae bien
				}
			}
		}
	}
	return w
}
