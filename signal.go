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
)

// Conexión con el servidor público gratuito de PeerJS. Lo usamos como relevo: cada paquete del
// juego viaja por el servidor hasta el otro PC. El servidor exige el formato de la librería
// PeerJS (si no, corta) y deja mandar ~10 mensajes por segundo.

const idPrefix = "idle-v1-"

func signalURL() string {
	if u := os.Getenv("MJ_SIGNAL"); u != "" { // para pruebas con un servidor propio
		return u
	}
	return "wss://0.peerjs.com:443/peerjs"
}

type sigMsg struct {
	Type    string          `json:"type"`
	Src     string          `json:"src,omitempty"`
	Dst     string          `json:"dst,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// sigPayload lleva un paquete del juego disfrazado de "candidato" de PeerJS.
type sigPayload struct {
	G []byte
}

var connID = fmt.Sprintf("dc_%x", rand.Int63())

const relayTag = "mj:"

type wireCand struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
}

type wirePayload struct {
	Candidate    *wireCand `json:"candidate,omitempty"`
	Type         string    `json:"type"`
	ConnectionID string    `json:"connectionId"`
}

func (p sigPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(wirePayload{Type: "data", ConnectionID: connID,
		Candidate: &wireCand{Candidate: relayTag + base64.StdEncoding.EncodeToString(p.G), SDPMid: "0"}})
}

func (p *sigPayload) UnmarshalJSON(b []byte) error {
	var w wirePayload
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.Candidate != nil && strings.HasPrefix(w.Candidate.Candidate, relayTag) {
		p.G, _ = base64.StdEncoding.DecodeString(strings.TrimPrefix(w.Candidate.Candidate, relayTag))
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
		if s.ctx.Err() == nil {
			logf("servidor: lectura cortada: %v", err)
		}
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		logf("servidor: mensaje raro: %.200s", b)
		return m, nil // se ignora y se sigue leyendo
	}
	if m.Type != "HEARTBEAT" && m.Type != "CANDIDATE" {
		logf("servidor: llega %s de %s", m.Type, m.Src)
	}
	return m, nil
}

func (s *signal) write(m sigMsg) {
	b, _ := json.Marshal(m)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	wctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := s.ws.Write(wctx, websocket.MessageText, b); err != nil && s.ctx.Err() == nil {
		logf("servidor: no se pudo enviar %s: %v", m.Type, err)
	}
}

func (s *signal) sendTo(dst string, p sigPayload) {
	b, _ := json.Marshal(p)
	s.write(sigMsg{Type: "CANDIDATE", Dst: dst, Payload: b})
}

func (s *signal) close() {
	s.cancel()
	s.ws.Close(websocket.StatusNormalClosure, "")
}
