package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

// Conexión por internet: el servidor público gratuito de PeerJS solo presenta a los dos PCs
// (intercambia los datos de conexión); después el juego va directo entre ellos con WebRTC.
// Si la red no deja ir directo, pasa por el servidor de relevo (TURN) de PeerJS.

const idPrefix = "idle-v1-"

func signalURL() string {
	if u := os.Getenv("MJ_SIGNAL"); u != "" { // para pruebas con un servidor propio
		return u
	}
	return "wss://0.peerjs.com:443/peerjs"
}

var rtcConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302", "stun:stun1.l.google.com:19302"}},
		{
			URLs: []string{
				"turn:eu-0.turn.peerjs.com:3478", "turn:us-0.turn.peerjs.com:3478",
				"turn:eu-0.turn.peerjs.com:3478?transport=tcp", "turn:us-0.turn.peerjs.com:3478?transport=tcp",
			},
			Username: "peerjs", Credential: "peerjsp",
		},
	},
}

type sigMsg struct {
	Type    string          `json:"type"`
	Src     string          `json:"src,omitempty"`
	Dst     string          `json:"dst,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// sigPayload es lo que viaja entre los dos por el servidor. El servidor público solo acepta
// (y si no, corta la conexión) el formato de la librería PeerJS, así que se traduce a ese formato.
type sigPayload struct {
	SDP       *webrtc.SessionDescription
	Candidate *webrtc.ICECandidateInit
	G         []byte // paquete del juego (modo relevo), viaja disfrazado de candidato
}

var connID = fmt.Sprintf("dc_%x", rand.Int63())

type wireCand struct {
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
	UsernameFragment string  `json:"usernameFragment,omitempty"`
}

type wirePayload struct {
	SDP           *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate     *wireCand                  `json:"candidate,omitempty"`
	Type          string                     `json:"type"`
	ConnectionID  string                     `json:"connectionId"`
	Label         string                     `json:"label,omitempty"`
	Serialization string                     `json:"serialization,omitempty"`
	Reliable      *bool                      `json:"reliable,omitempty"`
	Browser       string                     `json:"browser,omitempty"`
}

const relayTag = "mj:"

func (p sigPayload) MarshalJSON() ([]byte, error) {
	w := wirePayload{Type: "data", ConnectionID: connID}
	if p.SDP != nil {
		f := false
		w.SDP, w.Browser = p.SDP, "chrome"
		if p.SDP.Type == webrtc.SDPTypeOffer {
			w.Label, w.Serialization, w.Reliable = connID, "binary", &f
		}
	}
	mid, idx := "0", uint16(0)
	if p.Candidate != nil {
		c := p.Candidate
		w.Candidate = &wireCand{Candidate: c.Candidate, SDPMid: c.SDPMid, SDPMLineIndex: c.SDPMLineIndex}
		if c.UsernameFragment != nil {
			w.Candidate.UsernameFragment = *c.UsernameFragment
		}
		if w.Candidate.SDPMid == nil {
			w.Candidate.SDPMid = &mid
		}
		if w.Candidate.SDPMLineIndex == nil {
			w.Candidate.SDPMLineIndex = &idx
		}
	}
	if p.G != nil {
		w.Candidate = &wireCand{Candidate: relayTag + base64.StdEncoding.EncodeToString(p.G), SDPMid: &mid, SDPMLineIndex: &idx}
	}
	return json.Marshal(w)
}

func (p *sigPayload) UnmarshalJSON(b []byte) error {
	var w wirePayload
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	p.SDP = w.SDP
	if c := w.Candidate; c != nil {
		if strings.HasPrefix(c.Candidate, relayTag) {
			p.G, _ = base64.StdEncoding.DecodeString(strings.TrimPrefix(c.Candidate, relayTag))
		} else {
			p.Candidate = &webrtc.ICECandidateInit{Candidate: c.Candidate, SDPMid: c.SDPMid, SDPMLineIndex: c.SDPMLineIndex}
			if c.UsernameFragment != "" {
				u := c.UsernameFragment
				p.Candidate.UsernameFragment = &u
			}
		}
	}
	return nil
}

type signal struct {
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	wmu    sync.Mutex
}

// dialSignal se registra en el servidor con nuestro id. Devuelve taken=true si el id ya existe.
func dialSignal(id string) (s *signal, taken bool, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	url := fmt.Sprintf("%s?key=peerjs&id=%s&token=%x&version=1.5.4", signalURL(), id, rand.Int63())
	dctx, dcancel := context.WithTimeout(ctx, 10*time.Second)
	defer dcancel()
	ws, _, err := websocket.Dial(dctx, url, nil)
	if err != nil {
		cancel()
		logf("servidor: no conecta: %v", err)
		return nil, false, err
	}
	ws.SetReadLimit(1 << 20)
	s = &signal{ws: ws, ctx: ctx, cancel: cancel}
	m, err := s.read()
	if err != nil {
		s.close()
		return nil, false, err
	}
	logf("servidor: respuesta %s (id %s)", m.Type, id)
	if m.Type != "OPEN" {
		s.close()
		return nil, m.Type == "ID-TAKEN", fmt.Errorf("servidor: %s", m.Type)
	}
	go func() { // el servidor corta si no hay latidos
		for ctx.Err() == nil {
			time.Sleep(5 * time.Second)
			s.write(sigMsg{Type: "HEARTBEAT"})
		}
	}()
	return s, false, nil
}

func (s *signal) read() (sigMsg, error) {
	var m sigMsg
	_, b, err := s.ws.Read(s.ctx)
	if err != nil {
		logf("servidor: lectura cortada: %v", err)
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		logf("servidor: mensaje raro: %.200s", b)
		return m, nil // se ignora y se sigue leyendo
	}
	if m.Type != "HEARTBEAT" && !(m.Type == "CANDIDATE" && strings.Contains(string(m.Payload), relayTag)) {
		logf("servidor: llega %s de %s (%d bytes)", m.Type, m.Src, len(b))
	}
	return m, nil
}

func (s *signal) write(m sigMsg) {
	b, _ := json.Marshal(m)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	wctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.ws.Write(wctx, websocket.MessageText, b); err != nil {
		logf("servidor: no se pudo enviar %s: %v", m.Type, err)
	}
}

func (s *signal) sendTo(dst, typ string, p sigPayload) {
	b, _ := json.Marshal(p)
	s.write(sigMsg{Type: typ, Dst: dst, Payload: b})
}

func (s *signal) close() {
	s.cancel()
	s.ws.Close(websocket.StatusNormalClosure, "")
}

// newPC crea la conexión WebRTC y manda nuestros candidatos al otro a medida que aparecen.
func (n *Net) newPC(s *signal, dst string) (*webrtc.PeerConnection, error) {
	cfg := rtcConfig
	if os.Getenv("MJ_NODIRECT") != "" { // pruebas: simula una red que bloquea la conexión directa
		cfg.ICETransportPolicy = webrtc.ICETransportPolicyRelay
		cfg.ICEServers = nil
	}
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			logf("rtc: candidato propio %s %s", c.Typ, c.Protocol)
			ci := c.ToJSON()
			s.sendTo(dst, "CANDIDATE", sigPayload{Candidate: &ci})
		}
	})
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		logf("rtc: estado %s", st)
		if st == webrtc.PeerConnectionStateConnected {
			if pair, err := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair(); err == nil && pair != nil {
				logf("rtc: par elegido %s <-> %s", pair.Local.Typ, pair.Remote.Typ)
			}
		}
	})
	n.mu.Lock()
	n.pc = pc
	n.mu.Unlock()
	return pc, nil
}

// startRelay: a partir de ahora los paquetes del juego viajan por el servidor (siempre funciona
// si el servidor funciona, aunque con algo más de demora).
func (n *Net) startRelay(s *signal, to string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.relayOut != nil {
		return
	}
	n.relayTo, n.relayOut = to, make(chan []byte, 8)
	logf("relevo: activado hacia %s", to)
	go func(out chan []byte) {
		for msg := range out {
			if s.ctx.Err() != nil {
				return
			}
			s.sendTo(to, "CANDIDATE", sigPayload{G: msg})
		}
	}(n.relayOut)
}

func (n *Net) useDC(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		logf("rtc: canal abierto")
		n.mu.Lock()
		n.dc = dc
		n.mu.Unlock()
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) { n.onRTC(m.Data) })
}

// pending guarda candidatos que llegan antes de tener la descripción remota.
type pending struct {
	list []webrtc.ICECandidateInit
	set  bool
}

func (p *pending) add(pc *webrtc.PeerConnection, c webrtc.ICECandidateInit) {
	if pc == nil || !p.set {
		p.list = append(p.list, c)
		return
	}
	pc.AddICECandidate(c)
}

func (p *pending) flush(pc *webrtc.PeerConnection) {
	p.set = true
	for _, c := range p.list {
		pc.AddICECandidate(c)
	}
	p.list = nil
}

// rtcHost: registra la sala con su código y espera al compañero.
func (n *Net) rtcHost() {
	n.setRTC(NetConnecting)
	var s *signal
	for tries := 0; ; tries++ {
		n.mu.Lock()
		code := n.Code
		n.mu.Unlock()
		var taken bool
		var err error
		s, taken, err = dialSignal(idPrefix + strings.ToLower(code))
		if err == nil {
			break
		}
		if taken && tries < 5 {
			n.mu.Lock()
			n.Code = newCode() // ese código ya lo tiene otra sala: otro
			n.mu.Unlock()
			continue
		}
		if !n.isClosed() {
			n.setRTC(NetNoInternet)
		}
		return
	}
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		s.close()
		return
	}
	n.sig = s
	n.mu.Unlock()
	n.setRTC(NetOnline)

	var pc *webrtc.PeerConnection
	var guest string
	var pend pending
	for {
		m, err := s.read()
		if err != nil {
			return
		}
		var p sigPayload
		json.Unmarshal(m.Payload, &p)
		switch m.Type {
		case "OFFER":
			if pc != nil || p.SDP == nil {
				continue // ya hay un compañero
			}
			guest = m.Src
			if pc, err = n.newPC(s, guest); err != nil {
				return
			}
			pc.OnDataChannel(n.useDC)
			if pc.SetRemoteDescription(*p.SDP) != nil {
				continue
			}
			pend.flush(pc)
			ans, err := pc.CreateAnswer(nil)
			if err != nil || pc.SetLocalDescription(ans) != nil {
				continue
			}
			s.sendTo(guest, "ANSWER", sigPayload{SDP: pc.LocalDescription()})
		case "CANDIDATE":
			if p.G != nil {
				n.mu.Lock()
				if n.via == "" && len(p.G) >= 3 && p.G[2] == 'J' {
					n.via = "relay"
					logf("conectado por relevo")
				}
				ok := n.via == "relay" && (n.relayTo == "" || n.relayTo == m.Src)
				n.mu.Unlock()
				if ok {
					n.startRelay(s, m.Src)
					n.mu.Lock()
					n.handle(p.G)
					n.mu.Unlock()
				}
			} else if p.Candidate != nil && (guest == "" || m.Src == guest) {
				pend.add(pc, *p.Candidate)
			}
		}
	}
}

// rtcGuest: busca la sala por su código y se conecta.
func (n *Net) rtcGuest() {
	n.mu.Lock()
	host := idPrefix + strings.ToLower(n.Code)
	n.mu.Unlock()
	s, _, err := dialSignal(fmt.Sprintf("%sg%x", idPrefix, rand.Int63()))
	if err != nil {
		n.setRTC(NetNoInternet)
		return
	}
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		s.close()
		return
	}
	n.sig = s
	n.mu.Unlock()

	pc, err := n.newPC(s, host)
	if err != nil {
		n.setRTC(NetFailed)
		return
	}
	unordered, noRetry := false, uint16(0) // como UDP: lo viejo no sirve, mejor perderlo que esperar
	dc, err := pc.CreateDataChannel("mj", &webrtc.DataChannelInit{Ordered: &unordered, MaxRetransmits: &noRetry})
	if err != nil {
		n.setRTC(NetFailed)
		return
	}
	n.useDC(dc)
	offer, err := pc.CreateOffer(nil)
	if err != nil || pc.SetLocalDescription(offer) != nil {
		n.setRTC(NetFailed)
		return
	}
	s.sendTo(host, "OFFER", sigPayload{SDP: pc.LocalDescription()})
	logf("invitado: oferta enviada a %s", host)

	go func() { // si en 7 segundos no hay conexión directa, usar el servidor de relevo
		time.Sleep(7 * time.Second)
		n.mu.Lock()
		direct := n.dc != nil || n.closed || n.rtcSt == NetNoRoom
		if !direct {
			n.via = "relay"
		}
		n.mu.Unlock()
		if !direct {
			n.startRelay(s, host)
		}
	}()

	var pend pending
	for {
		m, err := s.read()
		if err != nil {
			return
		}
		var p sigPayload
		json.Unmarshal(m.Payload, &p)
		switch m.Type {
		case "ANSWER":
			logf("invitado: respuesta recibida")
			if p.SDP != nil && pc.SetRemoteDescription(*p.SDP) == nil {
				pend.flush(pc)
			}
		case "CANDIDATE":
			if p.G != nil {
				n.mu.Lock()
				if n.via == "relay" {
					n.handle(p.G)
				}
				n.mu.Unlock()
			} else if p.Candidate != nil {
				pend.add(pc, *p.Candidate)
			}
		case "EXPIRE":
			if m.Src == host {
				n.setRTC(NetNoRoom) // nadie tiene ese código
			}
		}
	}
}
