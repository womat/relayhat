package relay

import (
	"errors"
	"relayhat/pkg/app/config"
	"relayhat/pkg/app/raspberry"
	"relayhat/pkg/app/validate"
)

const (
	Off State = iota
	On
	Toggle
)

var (
	ErrPin          = errors.New("can't open pin")
	ErrUnknownState = errors.New("unknown stat")
	ErrNotFound     = errors.New("relay not found")
	ErrInvalidRelay = errors.New("relay: invalid")
)

type State int

type HTTPBody struct {
	GPIO        int    `json:"gpio"`
	Description string `json:"description"`
	State       string `json:"state" validate:"required"`
}

type AllHTTPBody map[string]HTTPBody

type Relay struct {
	description string
	line        *raspberry.Line
}

type Relays map[string]*Relay

type Handler struct {
	*raspberry.Chip
	Relays
}

func New() (*Handler, error) {
	chip, err := raspberry.Open()

	return &Handler{
			Chip:   chip,
			Relays: Relays{}},
		err
}

func (rs *Handler) Close() error {
	return rs.Chip.Close()
}

func (rs *Handler) Connect(config map[string]config.RelayConfig) (err error) {
	for name, cfg := range config {
		var r *Relay

		if r, err = rs.newRelay(cfg); err != nil {
			return
		}

		rs.Relays[name] = r
	}

	return
}

func (rs *Handler) newRelay(cfg config.RelayConfig) (*Relay, error) {
	var err error
	var r Relay
	if r.line, err = rs.Chip.NewPin(cfg.GPIO); err != nil {
		err = ErrPin
		return &r, err
	}

	r.line.Output()
	r.line.Low()
	r.description = cfg.Description

	return &r, err
}

func (r *Relay) On() (err error) {
	r.line.High()
	return nil
}

func (r *Relay) Off() (err error) {
	r.line.Low()
	return nil
}

func (r *Relay) Toggle() (err error) {
	r.line.Toggle()
	return
}

func (r *Relay) State() State {
	if r.line.Shadow() == raspberry.High {
		return On
	}
	return Off
}

// All returns all relays projects.
func (rs *Handler) All() (AllHTTPBody, error) {
	resp := AllHTTPBody{}
	for name, r := range rs.Relays {
		resp[name] = HTTPBody{
			GPIO:        r.line.Pin(),
			Description: r.description,
			State:       stateToString(r.State()),
		}
	}

	return resp, nil
}

// Get returns the relay identified by name.
// Get returns ErrNotFound if the relay is not found.
func (rs *Handler) Get(name string) (HTTPBody, error) {
	r, ok := rs.Relays[name]
	if !ok {
		return HTTPBody{}, ErrNotFound
	}

	return HTTPBody{
		GPIO:        r.line.Pin(),
		Description: r.description,
		State:       stateToString(r.State()),
	}, nil
}

// Update updates an existing project.
// Update returns ErrNotFound if the project is not found.
// Update returns an error if the project is not valid.
// Update sets Project.Id to Id to prevent project Id inconsistencies.
// Update returns the updated project.
func (rs *Handler) Update(name string, relay HTTPBody) (HTTPBody, error) {
	// validate relay
	if !validate.IsStructValid(relay) {
		return relay, ErrInvalidRelay
	}

	switch relay.State {
	case "on":
		_ = rs.Relays[name].On()
	case "off":
		_ = rs.Relays[name].Off()
	case "toggle":
		_ = rs.Relays[name].Toggle()
	default:
		return relay, ErrNotFound
	}

	return rs.Get(name)
}

func stateToString(s State) string {
	switch s {
	case On:
		return "on"
	case Off:
		return "off"
	}
	return ""
}
