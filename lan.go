package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"net"
	"os"
	"time"
)

// Conexión directa en la misma red (casi sin demora). Una vez conectados por el servidor, cada
// PC le cuenta al otro sus direcciones locales y se prueban mensajes directos. Si llegan en los
// dos sentidos, el juego pasa a ir directo (60 actualizaciones por segundo). Si la red no lo
// permite (muchas oficinas aíslan los equipos), se sigue por el servidor sin que se note.

type lanLink struct {
	udp       *net.UDPConn
	token     [8]byte
	peerToken [8]byte
	hasPeer   bool // ya sabemos el token del otro
	peer      *net.UDPAddr
	ok        bool // nos llegan sus respuestas: lo directo funciona desde acá
	peerOK    bool // el otro también confirmó que le funciona
	rx        time.Time
	probing   bool
}

// lanStart abre el puerto directo (con n.mu tomado).
func (n *Net) lanStart() {
	if n.lan.udp != nil || os.Getenv("MJ_NOLAN") != "" { // MJ_NOLAN: pruebas, simula una red que aísla los equipos
		return
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		logf("directo: no se pudo abrir el puerto: %v", err)
		return
	}
	n.lan.udp = c
	rand.Read(n.lan.token[:])
	go n.lanRead(c)
	go func() { // contarle al otro dónde estamos, hasta que funcione lo directo
		for i := 0; i < 40; i++ {
			time.Sleep(1500 * time.Millisecond)
			n.mu.Lock()
			stop := n.closed || n.lan.ok && n.lan.peerOK
			ready := n.peer != ""
			n.mu.Unlock()
			if stop {
				return
			}
			if ready {
				n.sendLanInfo()
			}
		}
	}()
}

func (n *Net) sendLanInfo() {
	n.mu.Lock()
	c, tok := n.lan.udp, n.lan.token
	n.mu.Unlock()
	if c == nil {
		return
	}
	b := append([]byte{}, tok[:]...)
	b = binary.LittleEndian.AppendUint16(b, uint16(c.LocalAddr().(*net.UDPAddr).Port))
	for _, ip := range localIPs() {
		b = append(b, ip...)
	}
	n.send('L', b)
}

// lanInfo: llegaron las direcciones del otro (con n.mu tomado).
func (n *Net) lanInfo(b []byte) {
	if len(b) < 10 || n.lan.udp == nil {
		return
	}
	copy(n.lan.peerToken[:], b[:8])
	n.lan.hasPeer = true
	port := int(binary.LittleEndian.Uint16(b[8:]))
	var addrs []*net.UDPAddr
	for i := 10; i+4 <= len(b); i += 4 {
		addrs = append(addrs, &net.UDPAddr{IP: net.IP(append([]byte{}, b[i:i+4]...)), Port: port})
	}
	if n.lan.probing || n.lan.ok && n.lan.peerOK || len(addrs) == 0 {
		return
	}
	n.lan.probing = true
	logf("directo: probando %v", addrs)
	go func(c *net.UDPConn, tok [8]byte) {
		defer func() { n.mu.Lock(); n.lan.probing = false; n.mu.Unlock() }()
		for i := 0; i < 80; i++ {
			n.mu.Lock()
			stop := n.closed || n.lan.ok && n.lan.peerOK
			probe := n.probe('P')
			n.mu.Unlock()
			if stop {
				return
			}
			for _, a := range addrs {
				c.WriteToUDP(probe, a)
			}
			time.Sleep(150 * time.Millisecond)
		}
		logf("directo: no hubo respuesta (la red no deja conectar los equipos entre sí)")
	}(n.lan.udp, n.lan.token)
}

func (n *Net) lanRead(c *net.UDPConn) {
	buf := make([]byte, 1500)
	for {
		k, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		pkt := buf[:k]
		n.mu.Lock()
		switch {
		case k == 10 && (pkt[0] == 'P' || pkt[0] == 'Q'):
			valid := n.lan.hasPeer && bytes.Equal(pkt[1:9], n.lan.peerToken[:])
			if pkt[0] == 'P' { // el otro nos alcanza: le contestamos
				c.WriteToUDP(n.probe('Q'), addr)
			} else if valid {
				if !n.lan.ok {
					logf("directo: nos llegan sus respuestas (%v)", addr)
				}
				n.lan.ok, n.lan.peer, n.lan.rx = true, addr, time.Now()
			}
			if valid && pkt[9] == 1 && !n.lan.peerOK {
				n.lan.peerOK = true
				logf("directo: ¡funciona en los dos sentidos!")
			}
		case k >= 3 && pkt[0] == magic && n.lan.hasPeer && (n.lan.peer == nil || addr.String() == n.lan.peer.String()):
			if n.lan.peer == nil {
				n.lan.peer = addr
			}
			n.lan.rx = time.Now()
			n.handle(pkt)
		}
		n.mu.Unlock()
	}
}

// probe arma un mensaje de prueba: tipo, nuestro token y si a nosotros ya nos funciona.
func (n *Net) probe(kind byte) []byte {
	ok := byte(0)
	if n.lan.ok {
		ok = 1
	}
	return append(append([]byte{kind}, n.lan.token[:]...), ok)
}

// lanActive: lo directo funciona en los dos sentidos y sigue vivo (con n.mu tomado).
func (n *Net) lanActive() bool {
	return n.lan.ok && n.lan.peerOK && n.lan.peer != nil && time.Since(n.lan.rx) < 3*time.Second
}

// Direct: true si el juego va por la conexión directa.
func (n *Net) Direct() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lanActive()
}

func localIPs() [][]byte {
	var out [][]byte
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip := ipn.IP.To4(); ip != nil {
					out = append(out, []byte(ip))
				}
			}
		}
	}
	return out
}
