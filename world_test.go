package main

import (
	"strings"
	"testing"
	"time"
)

func TestMundoDeterminista(t *testing.T) {
	day := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	a := simulate("IDLE", day, []string{"ANA"}, nil)
	b := simulate("IDLE", day, []string{"ANA", "LUIS"}, nil)
	for _, c := range cpus { // lo de los CPU no depende de qué jugadores conoce cada PC
		if a.Wins[c.ID] != b.Wins[c.ID] {
			t.Fatalf("%s: %d vs %d", c.ID, a.Wins[c.ID], b.Wins[c.ID])
		}
	}
	tot := 0
	for _, c := range cpus {
		tot += a.Wins[c.ID]
		t.Logf("%s %d", c.Name, a.Wins[c.ID])
	}
	if tot < 15*2 || tot > 15*3 {
		t.Fatalf("partidas raras: %d", tot)
	}
}

func TestDiario(t *testing.T) {
	c := Config{Av: Avatar{Name: "ANA", Color: 2}, Room: "IDLE", ID: "yo1"}
	now := time.Date(2026, 10, 25, 9, 1, 0, 0, time.Local)
	c.History = []MatchRec{{Day: "2026-10-24", Game: "pong", Opp: "cpu:karl", Won: true, Me: 7, Them: 3},
		{Day: "2026-10-24", Game: "pong", Opp: "cpu:karl", Won: false, Me: 5, Them: 7},
		{Day: "2026-10-24", Game: "pong", Opp: "abc", OppName: "LUIS", Won: true, Me: 7, Them: 6}}
	c.Bought = []Purchase{{Day: "2026-10-24", Item: "CORONA"}}
	for _, first := range []string{"", "2026-10-24"} {
		c.LastDiary = first
		d := buildDiary(&c, now)
		t.Logf("=== N.%d %s (%d paginas)", d.No, d.Date, len(d.Pages))
		if len(d.Pages) < 3 {
			t.Fatalf("muy pocas paginas: %d", len(d.Pages))
		}
		for _, p := range d.Pages {
			t.Logf("-- %s %s", p.Section, p.Head)
			check := func(s string) {
				if strings.ContainsAny(s, "ÁÉÍÓÚÑáéíóúñ{}") {
					t.Fatalf("tilde o marcador: %q", s)
				}
			}
			check(p.Head)
			check(p.Ad)
			for _, a := range p.Arts {
				check(a.Title + a.Text)
				t.Logf("   [%v] %s: %s", a.Photo != nil, a.Title, a.Text)
			}
			for _, r := range p.Results {
				t.Logf("   %s %d-%d %s", r.W, r.SW, r.SL, r.L)
			}
			if p.Ad != "" {
				t.Logf("   AVISO: %s", p.Ad)
			}
		}
	}
}
