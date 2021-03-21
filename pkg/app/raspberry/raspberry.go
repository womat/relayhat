package raspberry

import (
	"time"
)

// Edge represents the change in Line level that triggers an interrupt.
type Edge string

// Level represents the high (true) or low (false) level of a Pin.
type Level bool

const (
	// EdgeNone indicates no level transitions will trigger an interrupt
	EdgeNone Edge = "none"

	// EdgeRising indicates an interrupt is triggered when the Line transitions from low to high.
	EdgeRising Edge = "rising"

	// EdgeFalling indicates an interrupt is triggered when the Line transitions from high to low.
	EdgeFalling Edge = "falling"

	// EdgeBoth indicates an interrupt is triggered when the Line changes level.
	EdgeBoth Edge = "both"

	// Level of pin, High / Low
	Low  Level = false
	High Level = true
)

var lines map[int]*Line

func init() {
	lines = map[int]*Line{}
}

type Chip struct {
}

func (l *Line) SetBounceTime(t time.Duration) {
	l.bounceTime = t
}

func (l *Line) BounceTime() time.Duration {
	return l.bounceTime
}

func (l *Line) Unwatch() {
	l.gpioPin.Unwatch()
}

func (l *Line) Input() {
	l.gpioPin.Input()
}

func (l *Line) Output() {
	l.gpioPin.Output()
}

func (l *Line) PullUp() {
	l.gpioPin.PullUp()
}

func (l *Line) PullDown() {
	l.gpioPin.PullDown()
}

func (l *Line) Pin() int {
	return l.gpioPin.Pin()
}

func (l *Line) Read() Level {
	return Level(l.gpioPin.Read())
}

func (l *Line) High() {
	l.gpioPin.High()
}

func (l *Line) Low() {
	l.gpioPin.Low()
}

func (l *Line) Toggle() {
	l.gpioPin.Toggle()
}

func (l *Line) Shadow() Level {
	return Level(l.gpioPin.Shadow())
}

/*
func (pin *Line) Polling(edge Edge, ) {
	lastState := pin.gpioPin.Read()

	for ; true; <-time.After(500 * time.Millisecond) {
		if p := pin.gpioPin.Read(); p != lastState {
			debug.InfoLog.Printf("pin %v is %v\n", pin.Pin(), p)

			switch edge {
			case EdgeBoth:
				debug.InfoLog.Printf("pin %v switch from %v to %v\n", pin.Pin(), lastState, p)
			case EdgeFalling:
				if !p {
					debug.InfoLog.Printf("pin %v switch to (Low) %v\n", pin.Pin(), p)
				}
			case EdgeRising:
				if p {
					debug.InfoLog.Printf("pin %v switch to (High) %v\n", pin.Pin(), p)
				}
			}
			lastState = p
		}
	}
}
*/
