package main

import (
	"net"
	"time"
)

// Una sola mascota a la vez: si Idle ya está abierto, el segundo se cierra solo.
// Se reintenta un momento porque al actualizarse la versión vieja tarda un poco en cerrarse.

var instanceLock net.Listener

func acquireInstance() bool {
	deadline := time.Now().Add(3 * time.Second)
	for {
		l, err := net.Listen("tcp", "127.0.0.1:47811")
		if err == nil {
			instanceLock = l
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}
