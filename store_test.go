package main

import "testing"

func TestTienda(t *testing.T) {
	for _, c := range storeCats {
		if len(c.prices) != catCount(c.key) {
			t.Fatalf("%s: %d precios para %d cosas", c.key, len(c.prices), catCount(c.key))
		}
	}
	for _, b := range bodies {
		if len(b.rows) != 8 {
			t.Fatalf("%s: %d filas", b.name, len(b.rows))
		}
		for _, r := range b.rows {
			if len(r) != 12 {
				t.Fatalf("%s: fila %q", b.name, r)
			}
		}
	}
	for _, h := range hats {
		for _, r := range h.rows {
			if len(h.rows) != 3 || len(r) != 12 {
				t.Fatalf("%s: fila %q", h.name, r)
			}
		}
	}
	// con una victoria contra el CPU más fácil alcanza para algo
	c := Config{}
	got := c.Record(MatchRec{Game: "pong", Opp: "cpu:mochi", Won: true})
	cheap := 1 << 30
	for _, cat := range storeCats {
		for _, p := range cat.prices {
			if p > 0 && p < cheap {
				cheap = p
			}
		}
	}
	if got < cheap {
		t.Fatalf("una victoria da %d y lo más barato cuesta %d", got, cheap)
	}
}
