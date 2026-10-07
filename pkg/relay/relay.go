// Package relay switches a single relay through a Raspberry Pi GPIO output.
//
// It is hardware-facing only: a Relay knows its pin and its state, nothing of
// names, descriptions or configuration. New opens the pin through golib's
// gpio/rpi; NewWithPin takes any gpio.Pin, which is how the tests drive it with
// golib's in-memory gpio/rpiemu.
package relay

import (
	"errors"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpi"
)

const (
	Unknown State = iota - 1
	Off
	On
)

var (
	ErrUnknownState = errors.New("unknown state")
)

// State is the switching state of a relay.
type State int

// String returns the textual representation of the relay state.
func (s State) String() string {
	switch s {
	case On:
		return "on"
	case Off:
		return "off"
	}
	return "unknown"
}

// Relay is one relay driven by a GPIO output pin.
type Relay struct {
	gpioPin gpio.Pin
}

// New opens the given GPIO pin as output and returns a relay that is switched off.
func New(pin int) (*Relay, error) {
	p, err := rpi.NewPin(pin, rpi.WithMode(gpio.Output))
	if err != nil {
		return nil, err
	}

	r, err := NewWithPin(p)
	if err != nil {
		_ = p.Close()
		return nil, err
	}
	return r, nil
}

// NewWithPin returns a relay on an already opened output pin and switches it off.
// On error the pin is left open; closing it is up to the caller.
func NewWithPin(p gpio.Pin) (*Relay, error) {
	if err := p.SetValue(gpio.Low); err != nil {
		return nil, err
	}

	return &Relay{gpioPin: p}, nil
}

// GPIO returns the number of the relay's GPIO pin.
func (r *Relay) GPIO() int {
	return r.gpioPin.Number()
}

// Close releases the GPIO pin. The rpi backend reconfigures the line as input
// first, which switches the relay off.
func (r *Relay) Close() error {
	return r.gpioPin.Close()
}

// TurnOn switches the relay to the on state.
func (r *Relay) TurnOn() error {
	return r.gpioPin.SetValue(gpio.High)
}

// TurnOff switches the relay to the off state.
func (r *Relay) TurnOff() error {
	return r.gpioPin.SetValue(gpio.Low)
}

// GetState returns the relay's current state.
func (r *Relay) GetState() (State, error) {
	s, err := r.gpioPin.Value()
	if err != nil {
		return Unknown, err
	}

	switch s {
	case gpio.Low:
		return Off, nil
	case gpio.High:
		return On, nil
	}
	return Unknown, ErrUnknownState
}
