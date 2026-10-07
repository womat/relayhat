package relay

import (
	"testing"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpiemu"
)

// newTestRelay returns a relay on an emulated output pin.
func newTestRelay(t *testing.T, n int) (*Relay, rpiemu.Pin) {
	t.Helper()
	p, err := rpiemu.NewPin(n, rpiemu.WithMode(gpio.Output))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewWithPin(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, p
}

func TestNewSwitchesOff(t *testing.T) {
	p, err := rpiemu.NewPin(4, rpiemu.WithMode(gpio.Output))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetValue(gpio.High); err != nil {
		t.Fatal(err)
	}

	r, err := NewWithPin(p)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := r.GetState(); err != nil || s != Off {
		t.Errorf("GetState() = %v, %v; want off", s, err)
	}
	if r.GPIO() != 4 {
		t.Errorf("GPIO() = %d, want 4", r.GPIO())
	}
}

func TestTurnOnOff(t *testing.T) {
	r, p := newTestRelay(t, 17)

	if err := r.TurnOn(); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.GetState(); s != On {
		t.Errorf("after TurnOn state = %v, want on", s)
	}
	if l, _ := p.Value(); l != gpio.High {
		t.Errorf("after TurnOn pin = %v, want high", l)
	}

	if err := r.TurnOff(); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.GetState(); s != Off {
		t.Errorf("after TurnOff state = %v, want off", s)
	}
}

func TestNewWithPinRejectsInput(t *testing.T) {
	p, err := rpiemu.NewPin(22) // input by default, SetValue fails
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithPin(p); err == nil {
		t.Error("expected an error for a pin that is not an output")
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{On: "on", Off: "off", Unknown: "unknown", State(7): "unknown"} {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}
