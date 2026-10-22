package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// Experimento: qué servidores TURN públicos funcionan (conexión forzada por relevo TURN).
// Solo con MJ_TURN=1.
func TestServidoresTURN(t *testing.T) {
	if os.Getenv("MJ_TURN") == "" {
		t.Skip("solo con MJ_TURN=1")
	}
	cands := []webrtc.ICEServer{
		{URLs: []string{"turn:eu-0.turn.peerjs.com:3478"}, Username: "peerjs", Credential: "peerjsp"},
		{URLs: []string{"turn:eu-0.turn.peerjs.com:3478?transport=tcp"}, Username: "peerjs", Credential: "peerjsp"},
		{URLs: []string{"turn:us-0.turn.peerjs.com:3478"}, Username: "peerjs", Credential: "peerjsp"},
		{URLs: []string{"turn:openrelay.metered.ca:80"}, Username: "openrelayproject", Credential: "openrelayproject"},
		{URLs: []string{"turn:openrelay.metered.ca:443"}, Username: "openrelayproject", Credential: "openrelayproject"},
		{URLs: []string{"turn:openrelay.metered.ca:443?transport=tcp"}, Username: "openrelayproject", Credential: "openrelayproject"},
		{URLs: []string{"turn:staticauth.openrelay.metered.ca:443?transport=tcp"}, Username: "openrelayproject", Credential: "openrelayprojectsecret"},
		{URLs: []string{"turns:openrelay.metered.ca:443?transport=tcp"}, Username: "openrelayproject", Credential: "openrelayproject"},
	}
	for _, srv := range cands {
		cfg := webrtc.Configuration{ICEServers: []webrtc.ICEServer{srv}, ICETransportPolicy: webrtc.ICETransportPolicyRelay}
		a, _ := webrtc.NewPeerConnection(cfg)
		b, _ := webrtc.NewPeerConnection(cfg)
		a.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c != nil {
				b.AddICECandidate(c.ToJSON())
			}
		})
		b.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c != nil {
				a.AddICECandidate(c.ToJSON())
			}
		})
		open := make(chan bool, 1)
		b.OnDataChannel(func(dc *webrtc.DataChannel) {
			dc.OnMessage(func(webrtc.DataChannelMessage) { open <- true })
		})
		dc, _ := a.CreateDataChannel("x", nil)
		dc.OnOpen(func() { dc.SendText("hola") })
		offer, _ := a.CreateOffer(nil)
		a.SetLocalDescription(offer)
		b.SetRemoteDescription(offer)
		ans, _ := b.CreateAnswer(nil)
		b.SetLocalDescription(ans)
		a.SetRemoteDescription(ans)
		select {
		case <-open:
			t.Logf("FUNCIONA   %s", strings.Join(srv.URLs, " "))
		case <-time.After(15 * time.Second):
			t.Logf("no         %s", strings.Join(srv.URLs, " "))
		}
		a.Close()
		b.Close()
	}
}
