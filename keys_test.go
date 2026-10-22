package main

import (
	"testing"
	"time"
)

func chord(g *globalInput) (sw []bool) {
	sw = append(sw, g.onKey(KUp, true, false), g.onKey(KDown, true, false))
	sw = append(sw, g.onKey(KUp, false, false), g.onKey(KDown, false, false))
	return
}

func TestGestoYTeclas(t *testing.T) {
	g := &globalInput{down: map[Key]bool{}, swallowed: map[Key]bool{}}

	// modo trabajo: todo pasa
	if g.onKey('A', true, false) || g.onKey('A', false, false) {
		t.Fatal("en modo trabajo no se debe tragar nada")
	}
	// un solo ↑+↓ no cambia de modo y pasa
	if sw := chord(g); sw[0] || sw[1] || g.game {
		t.Fatalf("un solo gesto no debe activar: %v game=%v", sw, g.game)
	}
	// el segundo ↑+↓ seguido activa el modo juego; la tecla que lo completa se traga
	sw := chord(g)
	if !g.game {
		t.Fatal("dos gestos seguidos deben activar el modo juego")
	}
	if sw[0] || !sw[1] || sw[2] || !sw[3] {
		t.Fatalf("tragadas al activar: %v (↑ ya estaba abajo antes: pasa; ↓ completa el gesto: se traga)", sw)
	}
	// en modo juego: letras y flechas se tragan (bajar y soltar) y quedan para el juego
	if !g.onKey('A', true, false) || !g.onKey('A', false, false) {
		t.Fatal("en modo juego las teclas no deben llegar a la app")
	}
	if !g.onKey(KUp, true, false) || !g.onKey(KUp, false, false) {
		t.Fatal("las flechas deben ser del juego")
	}
	if keys, game := g.Frame(); !game || len(keys) != 2 || keys[0] != 'A' || keys[1] != KUp {
		t.Fatalf("el juego debe recibir A y ↑: %v %v", keys, game)
	}
	// atajos con ⌘/Ctrl/Alt siempre pasan
	if g.onKey('C', true, true) || g.onKey('C', false, true) {
		t.Fatal("⌘C debe llegar a la app")
	}
	// repetición automática de una tecla mantenida: no cuenta como nueva
	g.onKey(KDown, true, false)
	g.onKey(KDown, true, false)
	g.onKey(KDown, false, false)
	if keys, _ := g.Frame(); len(keys) != 1 {
		t.Fatalf("la repetición no debe contar como otra tecla: %v", keys)
	}
	// dos gestos separados por mucho tiempo no cuentan
	chord(g)
	g.lastChord = time.Now().Add(-2 * time.Second)
	chord(g)
	if !g.game {
		t.Fatal("gestos muy separados no deben salir del modo juego")
	}
	// salir: dos gestos seguidos; la tecla que lo completa también se traga
	g.lastChord = time.Time{}
	chord(g)
	if !g.game {
		t.Fatal("todavía falta el segundo gesto")
	}
	sw = chord(g)
	if g.game || !sw[1] {
		t.Fatalf("debe volver al modo trabajo tragando la tecla del gesto: game=%v %v", g.game, sw)
	}
	if g.onKey('A', true, false) || g.onKey('A', false, false) {
		t.Fatal("de vuelta en modo trabajo, las teclas pasan")
	}
}
