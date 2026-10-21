package main

import (
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// Sonidos retro generados al vuelo (onda cuadrada / diente de sierra), sin archivos.
const sampleRate = 44100

type tone struct {
	f, dur, vol float64
	saw         bool
	delay       float64
}

var sfxDefs = [EvCount][]tone{
	EvHit1:  {{f: 440, dur: .06, vol: .25}},
	EvHit2:  {{f: 523, dur: .06, vol: .25}},
	EvWall:  {{f: 240, dur: .04, vol: .25}},
	EvTick:  {{f: 330, dur: .05, vol: .25}},
	EvServe: {{f: 660, dur: .04, vol: .2}},
	EvPoint: {{f: 130, dur: .22, vol: .22, saw: true}},
	EvWin:   {{f: 523, dur: .1, vol: .25}, {f: 659, dur: .1, vol: .25, delay: .11}, {f: 784, dur: .25, vol: .25, delay: .22}},
}

type Sound struct {
	ctx  *audio.Context
	pcm  [EvCount][]byte
	live []*audio.Player // referencias para que no se corten a mitad
	Mute bool
}

func NewSound() *Sound {
	s := &Sound{}
	if os.Getenv("MJ_NOSOUND") != "" { // para pruebas en equipos sin parlantes
		return s
	}
	s.ctx = audio.NewContext(sampleRate)
	for i, def := range sfxDefs {
		s.pcm[i] = render(def)
	}
	return s
}

func render(tones []tone) []byte {
	total := 0.0
	for _, t := range tones {
		total = math.Max(total, t.delay+t.dur)
	}
	n := int(total * sampleRate)
	mix := make([]float64, n)
	for _, t := range tones {
		start := int(t.delay * sampleRate)
		ln := int(t.dur * sampleRate)
		for i := 0; i < ln && start+i < n; i++ {
			ph := math.Mod(float64(i)*t.f/sampleRate, 1)
			v := 1.0
			if t.saw {
				v = 2*ph - 1
			} else if ph > .5 {
				v = -1
			}
			env := math.Exp(-5 * float64(i) / float64(ln)) // se apaga rápido
			mix[start+i] += v * env * t.vol
		}
	}
	out := make([]byte, n*4) // 16 bits, estéreo
	for i, v := range mix {
		x := int16(clamp(v, -1, 1) * 32767)
		out[i*4], out[i*4+1] = byte(x), byte(x>>8)
		out[i*4+2], out[i*4+3] = byte(x), byte(x>>8)
	}
	return out
}

func (s *Sound) Play(ev int) {
	if s.Mute || s.ctx == nil || ev < 0 || ev >= EvCount {
		return
	}
	keep := s.live[:0]
	for _, p := range s.live {
		if p.IsPlaying() {
			keep = append(keep, p)
		}
	}
	p := s.ctx.NewPlayerFromBytes(s.pcm[ev])
	p.Play()
	s.live = append(keep, p)
}
