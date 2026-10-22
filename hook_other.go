//go:build !windows && !darwin

package main

// En otros sistemas (Linux, solo para pruebas) no hay teclado de todo el sistema:
// se usa el de la ventana o el guion de pruebas (MJ_SCRIPT).
func startHook() { gin.setStatus(HookOff) }
