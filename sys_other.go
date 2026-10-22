//go:build !windows && !darwin

package main

import "github.com/hajimehoshi/ebiten/v2"

// En Linux (solo para pruebas) se usa el mouse de la ventana.
func globalMouseDown() bool { return ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) }
func trackFocus()           {}
func restoreFocus()         {}
