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

type Relay struct {
	gpioPin gpio.Pin
}

// New creates a relay for the given GPIO pin and initializes it to off.
func New(pin int) (*Relay, error) {
	p, err := rpi.NewPin(pin, rpi.WithMode(gpio.Output))
	if err != nil {
		return nil, err
	}

	if err = p.SetValue(gpio.Low); err != nil {
		_ = p.Close()
		return nil, err
	}

	return &Relay{gpioPin: p}, nil
}

// Close releases the underlying GPIO pin resources.
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

// Toggle switches the relay to the opposite state and returns the new state.
func (r *Relay) Toggle() (State, error) {
	s, err := r.GetState()
	if err != nil {
		return Unknown, err
	}

	switch s {
	case Off:
		return On, r.gpioPin.SetValue(gpio.High)

	case On:
		return Off, r.gpioPin.SetValue(gpio.Low)
	}
	return Unknown, ErrUnknownState
}

// GetState returns the relay's current state.
func (r *Relay) GetState() (State, error) {
	s, err := r.gpioPin.Value()
	if err != nil {
		return Unknown, err
	}

	switch s {
	case gpio.Low:
		return Off, err
	case gpio.High:
		return On, err
	}
	return Unknown, ErrUnknownState
}
