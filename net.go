package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// Conexión entre los dos PCs. Se intentan dos caminos a la vez y gana el primero que funcione:
//   - "lan": UDP directo en la misma red (sin internet).
//   - "rtc": WebRTC por internet con código de sala (pasa por firewalls y WiFi de oficina).
//
// El que crea la sala calcula todo y manda el estado; el que se une manda su paleta.
const (
	GamePort = 47800
	DiscPort = 47801 // anuncios de sala por broadcast
	magic    = 'M'
	proto    = 5
	beaconID = "IDLE1|"
)

type Room struct {
	Name, Code string
	Addr       *net.UDPAddr
	Seen       time.Time
}

type StatePkt struct {
	A, B, X, Y, Tm, VX, VY float32
	S1, S2                 uint8
	Ph                     Phase
	Winner                 uint8
	Seq                    uint16
	Ev                     [EvCount]uint8
	Flags                  uint8  // 1 = pausado porque alguien se escondió
	Echo, Hold             uint16 // para medir la demora: reloj del invitado devuelto y cuánto lo retuve
}

const statLen = 7*4 + 4 + 2 + EvCount + 1 + 4

// RelayGap: por el servidor público se puede mandar ~10 mensajes por segundo (si no, corta).
const RelayGap = 140 * time.Millisecond

// Estado del camino por internet, para mostrarlo en pantalla.
const (
	NetIdle = iota
	NetConnecting
	NetOnline
	NetNoInternet
	NetNoRoom
	NetFailed
)

type Net struct {
	mu     sync.Mutex
	host   bool
	closed bool
	bye    bool
	lastRx time.Time
	via    string // "", "lan", "rtc" o "relay"
	seq    uint16
	lastTx time.Time

	// lan
	conn    *net.UDPConn
	disc    *net.UDPConn
	lanPeer *net.UDPAddr
	rooms   map[string]*Room

	// internet
	Code     string
	sig      *signal
	pc       *webrtc.PeerConnection
	dc       *webrtc.DataChannel
	relayTo  string      // por quién mandar por el servidor (modo relevo)
	relayOut chan []byte // cola de envío por el servidor, para no frenar el juego
	rtcSt    int
	rtcErr   string

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

func newCode() string {
	const A = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = A[rand.Intn(len(A))]
	}
	return string(b)
}

// ---------- crear sala ----------

func HostRoom() *Net {
	n := &Net{host: true, Code: newCode()}
	if c := os.Getenv("MJ_CODE"); len(c) == 4 { // solo para pruebas
		n.Code = c
	}
	if c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: GamePort}); err == nil {
		n.conn = c
	} else if c, err := net.ListenUDP("udp4", &net.UDPAddr{}); err == nil {
		n.conn = c // puerto ocupado: uno libre (se anuncia igual)
	}
	if n.conn != nil {
		go n.readLoop(n.conn)
		go n.beaconLoop()
	}
	go n.rtcHost()
	return n
}

func (n *Net) beaconLoop() {
	msg := []byte(beaconID + roomName() + "|" + n.Code)
	for {
		n.mu.Lock()
		stop := n.closed || n.via != ""
		n.mu.Unlock()
		if stop {
			return
		}
		for _, b := range broadcastAddrs() {
			n.conn.WriteToUDP(msg, &net.UDPAddr{IP: b, Port: DiscPort})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ---------- buscar salas y unirse ----------

func NewSearcher() *Net {
	n := &Net{rooms: map[string]*Room{}}
	d, err := net.ListenUDP("udp4", &net.UDPAddr{Port: DiscPort})
	if err == nil {
		n.disc = d
		go n.discLoop(d)
	}
	return n
}

func (n *Net) discLoop(d *net.UDPConn) {
	buf := make([]byte, 256)
	for {
		k, addr, err := d.ReadFromUDP(buf)
		if err != nil {
			return
		}
		parts := strings.Split(string(buf[:k]), "|")
		if len(parts) != 3 || parts[0]+"|" != beaconID {
			continue
		}
		name, code := parts[1], parts[2]
		n.mu.Lock()
		if r, ok := n.rooms[code]; ok && addr.IP.IsLoopback() && !r.Addr.IP.IsLoopback() {
			r.Seen = time.Now() // la misma sala llega por varias vías: me quedo con la IP de red
		} else {
			n.rooms[code] = &Room{Name: name, Code: code, Addr: addr, Seen: time.Now()}
		}
		n.mu.Unlock()
	}
}

// Rooms devuelve las salas vistas en los últimos 3 segundos.
func (n *Net) Rooms() []Room {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []Room
	for _, r := range n.rooms {
		if time.Since(r.Seen) < 3*time.Second {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// JoinLAN se une a una sala de la red local (o a una IP escrita a mano).
func (n *Net) JoinLAN(addr *net.UDPAddr) error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.conn, n.lanPeer, n.via = c, addr, "lan"
	n.mu.Unlock()
	go n.readLoop(c)
	go n.helloLoop()
	return nil
}

// JoinCode se une por internet con el código de 4 letras.
func (n *Net) JoinCode(code string) {
	n.mu.Lock()
	n.Code, n.via, n.rtcSt = code, "rtc", NetConnecting
	n.mu.Unlock()
	go n.rtcGuest()
	go n.helloLoop()
}

// helloLoop repite el saludo hasta recibir el primer estado del anfitrión.
func (n *Net) helloLoop() {
	for i := 0; i < 120; i++ {
		n.mu.Lock()
		done := n.closed || n.hasSt
		n.mu.Unlock()
		if done {
			return
		}
		n.send('J', nil)
		time.Sleep(250 * time.Millisecond)
	}
}

// ---------- recepción ----------

func (n *Net) readLoop(c *net.UDPConn) {
	buf := make([]byte, 512)
	for {
		k, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		n.mu.Lock()
		if n.host && n.via == "" && k >= 3 && buf[0] == magic && buf[1] == proto && buf[2] == 'J' {
			n.via, n.lanPeer = "lan", addr
			logf("conectado por red local desde %s", addr)
		}
		if n.via == "lan" && n.lanPeer != nil && addr.IP.Equal(n.lanPeer.IP) && addr.Port == n.lanPeer.Port {
			n.handle(buf[:k])
		}
		n.mu.Unlock()
	}
}

func (n *Net) onRTC(data []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.host && n.via == "" && len(data) >= 3 && data[2] == 'J' {
		n.via = "rtc"
		logf("conectado directo (webrtc)")
	}
	if n.via == "rtc" {
		n.handle(data)
	}
}

// handle procesa un paquete del compañero (con n.mu tomado).
func (n *Net) handle(buf []byte) {
	if len(buf) < 3 || buf[0] != magic || buf[1] != proto {
		return
	}
	b := buf[3:]
	n.lastRx = time.Now()
	switch buf[2] {
	case 'B':
		n.bye = true
	case 'A':
		n.peerAv = append([]byte(nil), b...)
	case 'H':
		if n.host && len(b) >= 10 {
			n.hit = HitPkt{Idx: b[0], Sum: b[1],
				Y: float64(math.Float32frombits(binary.LittleEndian.Uint32(b[2:]))),
				B: float64(math.Float32frombits(binary.LittleEndian.Uint32(b[6:])))}
			n.hasHit = true
		}
	case 'I':
		if n.host && len(b) >= 5 {
			n.guestB = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
			n.guestRematch = b[4]
			n.hasGuestB = true
			n.guestPanic = len(b) >= 6 && b[5]&1 != 0
			if len(b) >= 8 {
				n.guestMs, n.guestMsAt = binary.LittleEndian.Uint16(b[6:]), time.Now()
			}
		}
	case 'S':
		if !n.host && len(b) >= statLen {
			f := func(i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:])) }
			st := StatePkt{A: f(0), B: f(1), X: f(2), Y: f(3), Tm: f(4), VX: f(5), VY: f(6),
				S1: b[28], S2: b[29], Ph: Phase(b[30]), Winner: b[31], Seq: binary.LittleEndian.Uint16(b[32:])}
			copy(st.Ev[:], b[34:34+EvCount])
			st.Flags = b[34+EvCount]
			st.Echo = binary.LittleEndian.Uint16(b[35+EvCount:])
			st.Hold = binary.LittleEndian.Uint16(b[37+EvCount:])
			if !n.hasSt || int16(st.Seq-n.st.Seq) > 0 { // los paquetes viejos que llegan tarde no sirven
				n.st, n.hasSt = st, true
			}
		}
	}
}

// ---------- envío ----------

func (n *Net) send(kind byte, body []byte) {
	n.mu.Lock()
	via, c, p, dc, out := n.via, n.conn, n.lanPeer, n.dc, n.relayOut
	if via == "relay" && (kind == 'S' || kind == 'I') {
		if time.Since(n.lastTx) < RelayGap {
			n.mu.Unlock()
			return
		}
		n.lastTx = time.Now()
	}
	msg := append([]byte{magic, proto, kind}, body...)
	if via == "relay" && out != nil {
		select {
		case out <- msg:
		default: // cola llena: se descarta, llegará el siguiente
		}
	}
	n.mu.Unlock()
	switch {
	case via == "lan" && c != nil && p != nil:
		c.WriteToUDP(msg, p)
	case via == "rtc" && dc != nil && dc.ReadyState() == webrtc.DataChannelStateOpen:
		dc.Send(msg)
	}
}

func (n *Net) SendState(p *Pong, flags uint8) {
	b := make([]byte, statLen)
	for i, v := range []float64{p.A, p.B, p.X, p.Y, p.Tm, p.VX, p.VY} {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(float32(v)))
	}
	b[28], b[29], b[30], b[31] = uint8(p.S1), uint8(p.S2), uint8(p.Ph), uint8(p.Winner)
	n.mu.Lock()
	n.seq++
	binary.LittleEndian.PutUint16(b[32:], n.seq)
	n.mu.Unlock()
	copy(b[34:], p.Ev[:])
	b[34+EvCount] = flags
	n.mu.Lock()
	binary.LittleEndian.PutUint16(b[35+EvCount:], n.guestMs)
	binary.LittleEndian.PutUint16(b[37+EvCount:], uint16(time.Since(n.guestMsAt).Milliseconds()))
	n.mu.Unlock()
	n.send('S', b)
}

func (n *Net) SendInput(b float64, rematch, flags uint8) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint16(buf[6:], NowMs())
	binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(b)))
	buf[4], buf[5] = rematch, flags
	n.send('I', buf)
}

func (n *Net) SendAvatar(av Avatar) { n.send('A', av.Encode()) }

var clockStart = time.Now()

// NowMs: reloj propio en milisegundos (da la vuelta cada ~65 s; sirve para medir demoras cortas).
func NowMs() uint16 { return uint16(time.Since(clockStart).Milliseconds()) }

// Relayed: true si la conexión va por el servidor (pocas actualizaciones por segundo).
func (n *Net) Relayed() bool { return n.Via() == "relay" }

// HitPkt: el invitado avisa que golpeó la bola (Idx = golpes previos, Sum = puntos jugados).
type HitPkt struct {
	Idx, Sum uint8
	Y, B     float64
}

func (n *Net) SendHit(h HitPkt) {
	b := make([]byte, 10)
	b[0], b[1] = h.Idx, h.Sum
	binary.LittleEndian.PutUint32(b[2:], math.Float32bits(float32(h.Y)))
	binary.LittleEndian.PutUint32(b[6:], math.Float32bits(float32(h.B)))
	n.send('H', b)
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

// ---------- estado ----------

func (n *Net) Connected() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.via != "" && !n.lastRx.IsZero()
}

// Lost: el otro se fue o no responde hace rato.
func (n *Net) Lost() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.bye || (!n.lastRx.IsZero() && time.Since(n.lastRx) > 4*time.Second)
}

// Via: por dónde va la conexión ("lan", "rtc", "relay"), para mostrarlo.
func (n *Net) Via() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.via
}

func (n *Net) RTCStatus() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.rtcSt
}

func (n *Net) setRTC(st int) {
	n.mu.Lock()
	if n.rtcSt != NetOnline || st == NetFailed {
		n.rtcSt = st
	}
	n.mu.Unlock()
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

func (n *Net) Close() {
	if n == nil {
		return
	}
	for i := 0; i < 3; i++ {
		n.send('B', nil)
	}
	n.mu.Lock()
	n.closed = true
	conn, disc, sig, pc, out := n.conn, n.disc, n.sig, n.pc, n.relayOut
	n.relayOut = nil
	n.mu.Unlock()
	if out != nil {
		close(out)
	}
	if conn != nil {
		conn.Close()
	}
	if disc != nil {
		disc.Close()
	}
	if sig != nil {
		sig.close()
	}
	if pc != nil {
		go pc.Close()
	}
}

func (n *Net) isClosed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed
}

// ---------- utilidades de red ----------

func broadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast, net.IPv4(127, 0, 0, 1)}
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip, m := ipn.IP.To4(), ipn.Mask
			if len(m) == 16 {
				m = m[12:]
			}
			b := make(net.IP, 4)
			for k := range b {
				b[k] = ip[k] | ^m[k]
			}
			out = append(out, b)
		}
	}
	return out
}

// LocalIP: la IP de este PC en la red de casa/oficina, para mostrarla en pantalla.
func LocalIP() string {
	best := ""
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip := ipn.IP.To4()
			if ip.IsPrivate() {
				return ip.String()
			}
			if best == "" && !ip.IsLinkLocalUnicast() {
				best = ip.String()
			}
		}
	}
	return best
}

func roomName() string {
	h, _ := os.Hostname()
	h = strings.ToUpper(strings.TrimSuffix(strings.Split(h, ".")[0], "-local"))
	var b strings.Builder
	for _, r := range h {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		s = "PC"
	}
	if len(s) > 14 {
		s = s[:14]
	}
	return s
}
