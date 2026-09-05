package consolecodec

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/logocomune/gomeshcom-client/internal/callsign"
	"github.com/logocomune/gomeshcom-client/internal/meshcom"
)

const MaxFirmwarePayloadBytes = 160

var (
	ErrInvalidDestination = errors.New("invalid console destination")
	ErrCommandInjection   = errors.New("console message contains a command separator")
	ErrPayloadTooLong     = errors.New("console payload exceeds firmware limit")
	ErrSelfDirectMessage  = errors.New("direct message to local callsign is not allowed")
)

type Identity interface {
	Current() string
}

type TextCommand struct {
	Destination     string
	Message         string
	MaxMessageRunes int
}

type Encoder struct {
	identity Identity
}

func NewEncoder(identity Identity) *Encoder {
	return &Encoder{identity: identity}
}

func (e *Encoder) Encode(command TextCommand) ([]byte, error) {
	if err := meshcom.ValidateOutgoingText(command.Destination, command.Message, command.MaxMessageRunes); err != nil {
		return nil, err
	}
	if strings.ContainsAny(command.Message, "\r\n\x00") {
		return nil, ErrCommandInjection
	}

	destination, err := e.encodeDestination(command.Destination)
	if err != nil {
		return nil, err
	}
	payload := destination + command.Message
	if len([]byte(payload)) > MaxFirmwarePayloadBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLong, len([]byte(payload)))
	}
	return []byte("::" + payload + "\r\n"), nil
}

func (e *Encoder) encodeDestination(destination string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(destination))
	if normalized == "*" {
		return "", nil
	}
	if isDecimal(normalized) {
		channel, err := strconv.Atoi(normalized)
		if err != nil || channel < 1 || channel > 99999 {
			return "", ErrInvalidDestination
		}
		return "{" + strconv.Itoa(channel) + "}", nil
	}
	if !callsign.IsValid(normalized) {
		return "", ErrInvalidDestination
	}
	if e.isLocalCallsign(normalized) {
		return "", ErrSelfDirectMessage
	}
	return "{" + normalized + "}", nil
}

func (e *Encoder) isLocalCallsign(destination string) bool {
	if e.identity == nil {
		return false
	}
	return callsign.Normalize(e.identity.Current()) == destination
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
