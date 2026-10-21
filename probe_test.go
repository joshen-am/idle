package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// Experimento: qué mensajes acepta el servidor público sin cortar la conexión. Solo con MJ_PROBE=1.
func TestSignalProbe(t *testing.T) {
	if os.Getenv("MJ_PROBE") == "" {
		t.Skip("solo con MJ_PROBE=1")
	}
	pc, _ := webrtc.NewPeerConnection(webrtc.Configuration{})
	pc.CreateDataChannel("x", nil)
	offer, _ := pc.CreateOffer(nil)
	pc.SetLocalDescription(offer)
	<-webrtc.GatheringCompletePromise(pc)
	sdp := pc.LocalDescription()
	cid := fmt.Sprintf("dc_%x", rand.Int63())
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	variants := []struct {
		name string
		msgs []sigMsg
	}{
		{"solo heartbeat", []sigMsg{{Type: "HEARTBEAT"}}},
		{"offer minima", []sigMsg{{Type: "OFFER", Dst: "idle-v1-nadie", Payload: raw(map[string]any{"sdp": map[string]string{"type": "offer", "sdp": "x"}})}}},
		{"offer actual", []sigMsg{{Type: "OFFER", Dst: "idle-v1-nadie", Payload: raw(sigPayload{SDP: sdp})}}},
		{"offer formato oficial", []sigMsg{{Type: "OFFER", Dst: "idle-v1-nadie", Payload: raw(map[string]any{
			"sdp": sdp, "type": "data", "connectionId": cid, "label": cid, "serialization": "binary", "reliable": false, "browser": "chrome"})}}},
		{"candidate oficial", []sigMsg{{Type: "CANDIDATE", Dst: "idle-v1-nadie", Payload: raw(map[string]any{
			"candidate": map[string]any{"candidate": "candidate:1 1 udp 1 1.2.3.4 5 typ host", "sdpMid": "0", "sdpMLineIndex": 0}, "type": "data", "connectionId": cid})}}},
		{"g relevo", []sigMsg{{Type: "CANDIDATE", Dst: "idle-v1-nadie", Payload: raw(sigPayload{G: []byte{1, 2, 3}})}}},
	}
	for _, v := range variants {
		s, _, err := dialSignal(fmt.Sprintf("idle-v1-p%x", rand.Int63()))
		if err != nil {
			t.Logf("%-22s no conecta: %v", v.name, err)
			continue
		}
		closed := make(chan string, 1)
		go func() {
			for {
				m, err := s.read()
				if err != nil {
					closed <- err.Error()
					return
				}
				if m.Type != "" && m.Type != "HEARTBEAT" {
					t.Logf("%-22s   llega %s %.120s", v.name, m.Type, string(m.Payload))
				}
			}
		}()
		for _, m := range v.msgs {
			s.write(m)
		}
		select {
		case why := <-closed:
			t.Logf("%-22s CORTADA: %s", v.name, why)
		case <-time.After(3 * time.Second):
			t.Logf("%-22s ok (sigue abierta)", v.name)
		}
		s.close()
	}
}
