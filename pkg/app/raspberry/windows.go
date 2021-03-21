// +build windows

package raspberry

import (
	"fmt"
	"relayhat/pkg/app/raspberry/gpio"
	"time"
)

type Line struct {
	gpioPin *gpio.Pin
	handler func(*gpio.Pin)
	// the bounceTime defines the key bounce time (ms)
	// the value 0 ignores key bouncing
	bounceTime time.Duration
	// while bounceTimer is running, new signal are ignored (suppress key bouncing)
	bounceTimer *time.Timer
	edge        Edge
	lastLevel   gpio.Level
}

func Open() (*Chip, error) {
	if err := gpio.Open(); err != nil {
		return nil, err
	}

	lines = map[int]*Line{}
	return &Chip{}, nil
}

func (c *Chip) Close() error {
	return gpio.Close()
}

func (c *Chip) NewPin(p int) (*Line, error) {
	if _, ok := lines[p]; ok {
		return nil, fmt.Errorf("pin %v already used", p)
	}

	lines[p] = &Line{
		gpioPin:     gpio.NewPin(p),
		bounceTimer: time.NewTimer(0)}
	return lines[p], nil
}

func (l *Line) Watch(edge Edge, h func(*gpio.Pin)) error {
	l.handler = h
	l.edge = edge
	l.gpioPin.Mode()
	return l.gpioPin.Watch(gpio.Edge(edge), handler)
}

func (l *Line) TestPin(edge Edge) {
	switch {
	case l.edge == EdgeNone, edge == EdgeNone:
		return

	case edge == EdgeBoth:
		// if edge is EdgeBoth, handler is called twice
		if l.edge == EdgeBoth {
			handler(l.gpioPin)
		}

		if l.edge == EdgeBoth || l.edge == EdgeFalling || l.edge == EdgeRising {
			handler(l.gpioPin)
		}
	case edge == EdgeFalling:
		if l.edge == EdgeBoth || l.edge == EdgeFalling {
			handler(l.gpioPin)
		}
	case edge == EdgeRising:
		if l.edge == EdgeBoth || l.edge == EdgeRising {
			handler(l.gpioPin)
		}
	}
}

func handler(pin *gpio.Pin) {
	// check if map with pin struct exists
	l, ok := lines[pin.Pin()]
	if !ok {
		return
	}

	// if debounce is inactive, call handler function and returns
	if l.bounceTime == 0 {
		l.lastLevel = l.gpioPin.Read()
		l.handler(pin)
		return
	}

	select {
	case <-l.bounceTimer.C:
		// if bounce Timer is expired, accept new signals
		l.bounceTimer.Reset(l.bounceTime)
	default:
		// if bounce Timer is still running, ignore single
		return
	}

	go func(l *Line) {
		// wait until bounce Timer is expired and check if the pin has still the correct level
		// the correct level depends on the edge configuration
		<-l.bounceTimer.C
		l.bounceTimer.Reset(0)

		switch l.edge {
		case EdgeBoth:
			if l.gpioPin.Read() != l.lastLevel {
				l.lastLevel = l.gpioPin.Read()
				l.handler(pin)
			}
		case EdgeFalling:
			if !l.gpioPin.Read() {
				l.lastLevel = l.gpioPin.Read()
				l.handler(pin)
			}
		case EdgeRising:
			if l.gpioPin.Read() {
				l.lastLevel = l.gpioPin.Read()
				l.handler(pin)
			}
		}
	}(l)
}
