package main

import (
	"fmt"
	"time"
)

// Tienda: cuerpos, ojos, colores y gorros se compran con monedas (¤) que se ganan jugando.
// Lo básico es gratis; lo que ya tenías puesto antes de la tienda queda como tuyo.

type storeCat struct {
	key, label string
	prices     []int // precio de cada opción (0 = gratis)
}

var storeCats = []storeCat{
	{"body", "CUERPO", []int{0, 0, 150, 200, 250, 180}},
	{"eyes", "OJOS", []int{0, 0, 60, 80, 100, 150}},
	{"color", "COLOR", []int{0, 0, 0, 90, 90, 120, 120, 90, 250, 250, 200, 200}},
	{"hat", "GORRO", []int{0, 0, 150, 300, 120, 100, 140, 250, 300, 350, 200}},
}

func catCount(key string) int {
	switch key {
	case "body":
		return len(bodies)
	case "eyes":
		return len(eyeNames)
	case "color":
		return len(avColors)
	}
	return len(hats)
}

func itemName(key string, i int) string {
	switch key {
	case "body":
		return bodies[i].name
	case "eyes":
		return eyeNames[i]
	case "color":
		return avColors[i].name
	}
	return hats[i].name
}

// slot devuelve el campo del personaje que corresponde a la categoría.
func slot(av *Avatar, key string) *uint8 {
	switch key {
	case "body":
		return &av.Body
	case "eyes":
		return &av.Eyes
	case "color":
		return &av.Color
	}
	return &av.Hat
}

func price(key string, i int) int {
	for _, c := range storeCats {
		if c.key == key && i < len(c.prices) {
			return c.prices[i]
		}
	}
	return 0
}

func itemKey(key string, i int) string { return fmt.Sprintf("%s:%d", key, i) }

func (c *Config) Owns(key string, i int) bool {
	if price(key, i) == 0 {
		return true
	}
	for _, k := range c.Owned {
		if k == itemKey(key, i) {
			return true
		}
	}
	return false
}

// grandfather: lo que ya tenías puesto antes de que existiera la tienda es tuyo.
func (c *Config) grandfather() {
	for _, cat := range storeCats {
		i := int(*slot(&c.Av, cat.key))
		if !c.Owns(cat.key, i) {
			c.Owned = append(c.Owned, itemKey(cat.key, i))
		}
	}
}

// Buy compra (si alcanza) y se lo pone. Devuelve false si faltan monedas.
func (c *Config) Buy(key string, i int) bool {
	if !c.Owns(key, i) {
		p := price(key, i)
		if c.Coins < p {
			return false
		}
		c.Coins -= p
		c.Owned = append(c.Owned, itemKey(key, i))
		c.Bought = append(c.Bought, Purchase{Day: dayKey(time.Now()), Item: itemName(key, i)})
	}
	*slot(&c.Av, key) = uint8(i)
	return true
}

// nextOwned: la siguiente opción que tienes (para cambiarla con ◀ ▶ en Personaje).
func (c *Config) nextOwned(key string, d int) {
	s := slot(&c.Av, key)
	n := catCount(key)
	for k := 1; k <= n; k++ {
		i := ((int(*s)+d*k)%n + n) % n
		if c.Owns(key, i) {
			*s = uint8(i)
			return
		}
	}
}
