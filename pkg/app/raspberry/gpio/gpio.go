// +build windows

package gpio

// Level represents the high (true) or low (false) level of a Pin.
type Level bool

// Mode defines the IO mode of a Pin.
type Mode int

// Edge represents the change in Pin level that triggers an interrupt.
type Edge string

const (
	// EdgeNone indicates no level transitions will trigger an interrupt
	EdgeNone Edge = "none"

	// EdgeRising indicates an interrupt is triggered when the pin transitions from low to high.
	EdgeRising Edge = "rising"

	// EdgeFalling indicates an interrupt is triggered when the pin transitions from high to low.
	EdgeFalling Edge = "falling"

	// EdgeBoth indicates an interrupt is triggered when the pin changes level.
	EdgeBoth Edge = "both"

	// Level of pin, High / Low
	Low  Level = false
	High Level = true
)

// Pin Mode, a pin can be set in Input or Output mode
const (
	Input Mode = iota
	Output
)

type Pin struct {
	pin    int
	shadow Level
	mode   Mode
}

func Open() error {
	return nil
}

func Close() error {
	return nil
}

func NewPin(p int) *Pin {
	return &Pin{
		pin:    p,
		shadow: Low,
		mode:   Input,
	}
}

func (p *Pin) Watch(edge Edge, handler func(*Pin)) error {
	return nil
}

func (p *Pin) Mode() Mode {
	return p.mode
}

func (p *Pin) Unwatch() {
}

func (p *Pin) Input() {
	p.mode = Input
}

func (p *Pin) Output() {
	p.mode = Output
}

func (p *Pin) PullUp() {
}

func (p *Pin) PullDown() {
}

func (p *Pin) Pin() int {
	return p.pin
}

func (p *Pin) Read() Level {
	return p.shadow
}

func (p *Pin) High() {
	p.shadow = High
}

func (p *Pin) Low() {
	p.shadow = Low
}

func (p *Pin) Toggle() {
	p.shadow = !p.shadow
}

func (p *Pin) Shadow() Level {
	return p.shadow
}
