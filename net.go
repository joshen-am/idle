package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Juego en red por el servidor público (ver signal.go). No hay códigos: los dos entran a la
// misma sala por su nombre; el primero que llega la crea (anfitrión) y el segundo se une.
// El anfitrión calcula la partida; el invitado manda su paleta y sus golpes.
const (
	magic = 'M'
	proto = 6
	// RelayGap: el servidor corta si se mandan más de ~10 mensajes por segundo.
	RelayGap = 140 * time.Millisecond
)

// Estado de la búsqueda, para mostrarlo en pantalla.
const (
	NetConnecting = iota
	NetWaiting    // sala creada, esperando al compañero
	NetJoining    // la sala existe, entrando
	NetNoInternet // no se llega al servidor (se reintenta solo)
)

type StatePkt struct {
	A, B, X, Y, Tm   float32
	AX, AY, AVX, AVY float32 // último golpe o saque: desde ahí la bola es predecible
	S1, S2           uint8
	Ph               Phase
	Winner           uint8
	Seq              uint16
	Ev               [EvCount]uint8
	Flags            uint8  // 1 = pausado porque alguien se escondió
	Echo, Hold       uint16 // para medir la demora: reloj del invitado devuelto y cuánto lo retuve
	ASeq             uint8
	AAge             uint16 // ms desde el último golpe o saque
}

const statLen = 9*4 + 4 + 2 + EvCount + 1 + 4 + 1 + 2

// HitPkt: el invitado avisa que golpeó la bola (Idx = golpes previos, Sum = puntos jugados).
type HitPkt struct {
	Idx, Sum uint8
	Y, B     float64
}

type Net struct {
	mu        sync.Mutex
	room      string
	host      bool
	status    int
	closed    bool
	bye       bool
	connected bool
	lastRx    time.Time
	lastTx    time.Time
	seq       uint16

	sig  *signal
	peer string      // id del otro en el servidor
	out  chan []byte // cola de envío, para no frenar el juego
	lan  lanLink     // conexión directa en la misma red (lan.go)

	peerAv []byte // personaje del otro

	// lado anfitrión
	hit          HitPkt
	hasHit       bool
	guestPanic   bool
	guestB       float64
	guestMs      uint16
	guestMsAt    time.Time
	guestRematch uint8
	hasGuestB    bool

	// lado invitado
	st    StatePkt
	hasSt bool
}

// AutoRoom entra a la sala (la crea si no existe) y se queda intentando hasta encontrar al otro.
func AutoRoom(room string) *Net {
	n := &Net{room: room}
	go n.run()
	return n
}

func roomID(room string) string { return idPrefix + "r-" + strings.ToLower(room) }

func (n *Net) run() {
	hostID := roomID(n.room)
	for !n.isClosed() {
		n.setStatus(NetConnecting)
		s, taken, err := dialSignal(hostID)
		switch {
		case err == nil:
			n.asHost(s)
		case taken:
			if g, _, err := dialSignal(fmt.Sprintf("%sg%x", idPrefix, rand.Int63())); err == nil {
				n.asGuest(g, hostID)
			}
		default:
			n.setStatus(NetNoInternet)
		}
		n.mu.Lock()
		done := n.closed || n.connected // si ya jugaban y se cortó, lo maneja Lost()
		n.mu.Unlock()
		if done {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// asHost: la sala es nuestra; esperamos el saludo del compañero.
func (n *Net) asHost(s *signal) {
	if !n.attach(s, true, "") {
		return
	}
	n.setStatus(NetWaiting)
	defer s.close()
	for {
		m, err := s.read()
		if err != nil {
			return
		}
		if m.Type != "CANDIDATE" {
			continue
		}
		var p sigPayload
		if p.UnmarshalJSON(m.Payload) != nil || p.G == nil {
			continue
		}
		n.mu.Lock()
		if n.peer == "" && len(p.G) >= 3 && p.G[2] == 'J' {
			n.peer, n.connected = m.Src, true
			logf("red: entró el compañero (%s)", m.Src)
		}
		if m.Src == n.peer {
			n.handle(p.G)
		}
		n.mu.Unlock()
	}
}

// asGuest: la sala ya existe; saludamos al anfitrión hasta que conteste.
func (n *Net) asGuest(s *signal, hostID string) {
	if !n.attach(s, false, hostID) {
		return
	}
	n.setStatus(NetJoining)
	defer s.close()
	go func() {
		for i := 0; i < 40; i++ {
			n.mu.Lock()
			done := n.closed || n.hasSt
			n.mu.Unlock()
			if done {
				return
			}
			n.send('J', nil)
			time.Sleep(300 * time.Millisecond)
		}
	}()
	for {
		m, err := s.read()
		if err != nil {
			return
		}
		if m.Type == "EXPIRE" && m.Src == hostID {
			logf("red: la sala quedó vacía, se vuelve a intentar")
			return
		}
		var p sigPayload
		if m.Type != "CANDIDATE" || m.Src != hostID || p.UnmarshalJSON(m.Payload) != nil || p.G == nil {
			continue
		}
		n.mu.Lock()
		n.connected = true
		n.handle(p.G)
		n.mu.Unlock()
	}
}

// attach conecta el envío a este servidor.
func (n *Net) attach(s *signal, host bool, peer string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		s.close()
		return false
	}
	n.sig, n.host, n.peer, n.bye, n.hasSt, n.connected = s, host, peer, false, false, false
	n.lanStart()
	if n.out != nil {
		close(n.out)
	}
	n.out = make(chan []byte, 8)
	go func(out chan []byte) {
		for msg := range out {
			n.mu.Lock()
			to := n.peer
			n.mu.Unlock()
			if s.ctx.Err() != nil {
				return
			}
			if to == "" {
				continue
			}
			if testDelay > 0 { // pruebas: simula la demora de una red lenta
				m := msg
				time.AfterFunc(testDelay, func() { s.sendTo(to, sigPayload{G: m}) })
				continue
			}
			s.sendTo(to, sigPayload{G: msg})
		}
	}(n.out)
	return true
}

// handle procesa un paquete del compañero (con n.mu tomado).
func (n *Net) handle(buf []byte) {
	if len(buf) < 3 || buf[0] != magic || buf[1] != proto {
		return
	}
	b := buf[3:]
	n.lastRx = time.Now()
	f := func(i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[i:])) }
	switch buf[2] {
	case 'B':
		n.bye = true
	case 'L':
		n.lanInfo(b)
	case 'A':
		n.peerAv = append([]byte(nil), b...)
	case 'H':
		if n.host && len(b) >= 10 {
			n.hit = HitPkt{Idx: b[0], Sum: b[1], Y: float64(f(2)), B: float64(f(6))}
			n.hasHit = true
		}
	case 'I':
		if n.host && len(b) >= 8 {
			n.guestB, n.guestRematch, n.hasGuestB = float64(f(0)), b[4], true
			n.guestPanic = b[5]&1 != 0
			n.guestMs, n.guestMsAt = binary.LittleEndian.Uint16(b[6:]), time.Now()
		}
	case 'S':
		if !n.host && len(b) >= statLen {
			st := StatePkt{A: f(0), B: f(4), X: f(8), Y: f(12), Tm: f(16), AX: f(20), AY: f(24), AVX: f(28), AVY: f(32),
				S1: b[36], S2: b[37], Ph: Phase(b[38]), Winner: b[39], Seq: binary.LittleEndian.Uint16(b[40:])}
			copy(st.Ev[:], b[42:42+EvCount])
			i := 42 + EvCount
			st.Flags = b[i]
			st.Echo = binary.LittleEndian.Uint16(b[i+1:])
			st.Hold = binary.LittleEndian.Uint16(b[i+3:])
			st.ASeq = b[i+5]
			st.AAge = binary.LittleEndian.Uint16(b[i+6:])
			if !n.hasSt || int16(st.Seq-n.st.Seq) > 0 { // los paquetes viejos que llegan tarde no sirven
				n.st, n.hasSt = st, true
			}
		}
	}
}

// ---------- envío ----------

func (n *Net) send(kind byte, body []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lanActive() && kind != 'L' { // directo: sin límite de mensajes
		n.lan.udp.WriteToUDP(append([]byte{magic, proto, kind}, body...), n.lan.peer)
		return
	}
	if n.out == nil || n.peer == "" {
		return
	}
	if kind == 'S' || kind == 'I' {
		if time.Since(n.lastTx) < RelayGap {
			return
		}
		n.lastTx = time.Now()
	}
	select {
	case n.out <- append([]byte{magic, proto, kind}, body...):
	default: // cola llena: se descarta, llegará el siguiente
	}
}

func put32(b []byte, v float64) { binary.LittleEndian.PutUint32(b, math.Float32bits(float32(v))) }

func (n *Net) SendState(p *Pong, flags uint8) {
	b := make([]byte, statLen)
	for i, v := range []float64{p.A, p.B, p.X, p.Y, p.Tm, p.AX, p.AY, p.AVX, p.AVY} {
		put32(b[i*4:], v)
	}
	b[36], b[37], b[38], b[39] = uint8(p.S1), uint8(p.S2), uint8(p.Ph), uint8(p.Winner)
	copy(b[42:], p.Ev[:])
	i := 42 + EvCount
	b[i] = flags
	b[i+5] = p.ASeq
	binary.LittleEndian.PutUint16(b[i+6:], uint16(math.Min(65000, (p.T-p.AT)*1000)))
	n.mu.Lock()
	n.seq++
	binary.LittleEndian.PutUint16(b[40:], n.seq)
	binary.LittleEndian.PutUint16(b[i+1:], n.guestMs)
	binary.LittleEndian.PutUint16(b[i+3:], uint16(time.Since(n.guestMsAt).Milliseconds()))
	n.mu.Unlock()
	n.send('S', b)
}

func (n *Net) SendInput(b float64, rematch, flags uint8) {
	buf := make([]byte, 8)
	put32(buf, b)
	buf[4], buf[5] = rematch, flags
	binary.LittleEndian.PutUint16(buf[6:], NowMs())
	n.send('I', buf)
}

func (n *Net) SendAvatar(av Avatar) { n.send('A', av.Encode()) }

func (n *Net) SendHit(h HitPkt) {
	b := make([]byte, 10)
	b[0], b[1] = h.Idx, h.Sum
	put32(b[2:], h.Y)
	put32(b[6:], h.B)
	n.send('H', b)
}

var clockStart = time.Now()

var testDelay = func() time.Duration {
	ms, _ := strconv.Atoi(os.Getenv("MJ_DELAY_MS"))
	return time.Duration(ms) * time.Millisecond
}()

// NowMs: reloj propio en milisegundos (da la vuelta cada ~65 s; sirve para medir demoras cortas).
func NowMs() uint16 { return uint16(time.Since(clockStart).Milliseconds()) }

// ---------- estado ----------

func (n *Net) setStatus(s int) {
	n.mu.Lock()
	n.status = s
	n.mu.Unlock()
}

func (n *Net) Status() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

func (n *Net) IsHost() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.host
}

func (n *Net) Connected() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.connected && !n.lastRx.IsZero()
}

// Lost: el otro se fue o no responde hace rato.
func (n *Net) Lost() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.bye || (!n.lastRx.IsZero() && time.Since(n.lastRx) > 5*time.Second)
}

func (n *Net) TakeHit() (HitPkt, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	h, ok := n.hit, n.hasHit
	n.hasHit = false
	return h, ok
}

func (n *Net) PeerAvatar() (Avatar, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return DecodeAvatar(n.peerAv)
}

func (n *Net) GuestPanic() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.guestPanic
}

func (n *Net) GuestPaddle() (float64, uint8, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.guestB, n.guestRematch, n.hasGuestB
}

func (n *Net) State() (StatePkt, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.st, n.hasSt
}

func (n *Net) isClosed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed
}

func (n *Net) Close() {
	if n == nil {
		return
	}
	n.send('B', nil)
	time.Sleep(30 * time.Millisecond) // que alcance a salir el aviso
	n.mu.Lock()
	n.closed = true
	sig, out, udp := n.sig, n.out, n.lan.udp
	n.out = nil
	n.mu.Unlock()
	if udp != nil {
		udp.Close()
	}
	if out != nil {
		close(out)
	}
	if sig != nil {
		sig.close()
	}
}
