//go:build darwin

package main

import (
	"os"
	"testing"
)

// En las máquinas de prueba no se puede dar el permiso de Accesibilidad: se comprueba que las
// funciones del sistema cargan, que se detecta que falta el permiso y que nada se cae.
func TestHookMac(t *testing.T) {
	if os.Getenv("MJ_HOOKTEST") == "" {
		t.Skip("solo con MJ_HOOKTEST=1")
	}
	cf, ax, err := loadMacLibs()
	if err != nil {
		t.Fatalf("no cargaron las librerías: %v", err)
	}
	t.Logf("¿permiso de Accesibilidad?: %v", axIsProcessTrusted())
	if symValue(cf, "kCFRunLoopCommonModes") == 0 || symValue(ax, "kAXTrustedCheckOptionPrompt") == 0 || symValue(cf, "kCFBooleanTrue") == 0 {
		t.Fatal("faltan constantes del sistema")
	}
	tap := cgEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionDefault,
		1<<kCGEventKeyDown|1<<kCGEventKeyUp, 0, 0)
	t.Logf("event tap sin permiso: %v (0 = macOS no lo deja, lo esperado)", tap)
	if k, ok := macKey(126); !ok || k != KUp {
		t.Fatal("la flecha arriba debe ser KUp")
	}
	startHook() // pide el permiso y espera; no debe caerse
}
