package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Registro de conexión (registro.txt junto a la configuración), para averiguar qué falla.
var logMu sync.Mutex

func logPath() string {
	p := cfgPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "registro.txt")
}

func logf(format string, args ...any) {
	if os.Getenv("MJ_LOG_STDOUT") != "" { // pruebas: ver el registro en la consola
		fmt.Printf("%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	}
	p := logPath()
	if p == "" {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if st, err := os.Stat(p); err == nil && st.Size() > 512*1024 {
		os.Remove(p) // que no crezca para siempre
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
}
