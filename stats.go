package main

import (
	"crypto/rand"
	hexenc "encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// Victorias, monedas y ranking. Cada PC guarda sus partidas; cuando juegan online se pasan
// sus totales (perfil) para que el ranking muestre a los dos.

type MatchRec struct {
	Day     string // 2006-01-02
	Game    string // "pong"
	Opp     string // id del rival (un CPU o el id del amigo)
	OppName string
	Won     bool
	Me      int
	Them    int
}

type Purchase struct {
	Day  string
	Item string
}

type Friend struct {
	ID   string
	Name string
	Av   Avatar
	Wins map[string]int
	Seen string
}

// Profile: lo que se le cuenta al otro jugador al conectarse.
type Profile struct {
	ID   string
	Name string
	Wins map[string]int
}

var gameNames = map[string]string{"pong": "PONG"}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hexenc.EncodeToString(b)
}

// Wins: mis victorias por juego.
func (c *Config) Wins() map[string]int {
	w := map[string]int{}
	for _, m := range c.History {
		if m.Won {
			w[m.Game]++
		}
	}
	return w
}

func total(w map[string]int) int {
	n := 0
	for _, v := range w {
		n += v
	}
	return n
}

// Record anota una partida terminada y entrega monedas. Devuelve cuántas.
func (c *Config) Record(m MatchRec) int {
	m.Day = dayKey(time.Now())
	c.History = append(c.History, m)
	if len(c.History) > 2000 {
		c.History = c.History[len(c.History)-2000:]
	}
	coins := 5
	switch {
	case m.Won && cpuByID(m.Opp) != nil:
		coins = 20 + int(cpuByID(m.Opp).Skill*30)
	case m.Won:
		coins = 50
	case cpuByID(m.Opp) == nil:
		coins = 15
	}
	c.Coins += coins
	return coins
}

func (c *Config) MyProfile() Profile {
	return Profile{ID: c.ID, Name: c.Av.DisplayName(), Wins: c.Wins()}
}

func encodeProfile(p Profile) []byte {
	b, _ := json.Marshal(p)
	return b
}

// SeeFriend guarda lo último que se sabe del otro jugador.
func (c *Config) SeeFriend(p Profile, av Avatar) {
	if p.ID == "" || p.ID == c.ID {
		return
	}
	if c.Friends == nil {
		c.Friends = map[string]*Friend{}
	}
	c.Friends[p.ID] = &Friend{ID: p.ID, Name: p.Name, Av: av, Wins: p.Wins, Seen: dayKey(time.Now())}
}

type BoardRow struct {
	ID, Name string
	Av       Avatar
	Wins     map[string]int
	Total    int
	Me, CPU  bool
}

// Leaderboard: tú, tus amigos y los CPU, ordenados por victorias.
func (c *Config) Leaderboard(w *World) []BoardRow { return c.leaderboardAt(w, "") }

// leaderboardAt: el ranking contando solo las partidas hasta el día upTo ("" = todas).
func (c *Config) leaderboardAt(w *World, upTo string) []BoardRow {
	mine := map[string]int{}
	vsCPU := map[string]map[string]int{}
	for _, m := range c.History {
		if upTo != "" && m.Day > upTo {
			continue
		}
		if m.Won {
			mine[m.Game]++
		} else if cpuByID(m.Opp) != nil { // también cuentan las veces que te ganó a ti
			if vsCPU[m.Opp] == nil {
				vsCPU[m.Opp] = map[string]int{}
			}
			vsCPU[m.Opp][m.Game]++
		}
	}
	rows := []BoardRow{{ID: c.ID, Name: c.Av.DisplayName(), Av: c.Av, Wins: mine, Me: true}}
	for _, f := range c.Friends {
		rows = append(rows, BoardRow{ID: f.ID, Name: f.Name, Av: f.Av, Wins: f.Wins})
	}
	for _, cpu := range cpus {
		wins := map[string]int{"pong": w.Wins[cpu.ID]}
		for g, n := range vsCPU[cpu.ID] {
			wins[g] += n
		}
		rows = append(rows, BoardRow{ID: cpu.ID, Name: cpu.Name, Av: cpu.Av, Wins: wins, CPU: true})
	}
	for i := range rows {
		rows[i].Total = total(rows[i].Wins)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return !rows[i].CPU && rows[j].CPU // empate: primero las personas
	})
	return rows
}
