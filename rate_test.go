package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"
)

// Experimento: cuántos mensajes por segundo aguanta el servidor público antes de cortar.
// Solo con MJ_RATE=n (mensajes por segundo).
func TestServidorRitmo(t *testing.T) {
	rate, _ := strconv.Atoi(os.Getenv("MJ_RATE"))
	if rate == 0 {
		t.Skip("solo con MJ_RATE")
	}
	a, _, err := dialSignal(fmt.Sprintf("idle-v1-ra%x", rand.Int63()))
	if err != nil {
		t.Fatal(err)
	}
	bID := fmt.Sprintf("idle-v1-rb%x", rand.Int63())
	b, _, err := dialSignal(bID)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan string, 2)
	got := 0
	go func() {
		for {
			if _, _, err := b.ws.Read(b.ctx); err != nil {
				closed <- "b: " + err.Error()
				return
			}
			got++
		}
	}()
	go func() {
		for {
			if _, _, err := a.ws.Read(a.ctx); err != nil {
				closed <- "a: " + err.Error()
				return
			}
		}
	}()
	start := time.Now()
	sent := 0
	tk := time.NewTicker(time.Second / time.Duration(rate))
	for time.Since(start) < 90*time.Second {
		select {
		case why := <-closed:
			t.Logf("ritmo %d/s: CORTADA a los %.1fs tras %d enviados, %d recibidos (%s)", rate, time.Since(start).Seconds(), sent, got, why)
			return
		case <-tk.C:
			a.sendTo(bID, "CANDIDATE", sigPayload{G: make([]byte, 50)})
			sent++
		}
	}
	t.Logf("ritmo %d/s: aguantó 90s, %d enviados, %d recibidos", rate, sent, got)
}
