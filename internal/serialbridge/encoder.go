package serialbridge

import "github.com/logocomune/gomeshcom-client/internal/consolecodec"

const MaxFirmwarePayloadBytes = consolecodec.MaxFirmwarePayloadBytes

var (
	ErrInvalidDestination = consolecodec.ErrInvalidDestination
	ErrCommandInjection   = consolecodec.ErrCommandInjection
	ErrPayloadTooLong     = consolecodec.ErrPayloadTooLong
	ErrSelfDirectMessage  = consolecodec.ErrSelfDirectMessage
)

type Identity = consolecodec.Identity
type TextCommand = consolecodec.TextCommand
type Encoder = consolecodec.Encoder

func NewEncoder(identity Identity) *Encoder {
	return consolecodec.NewEncoder(identity)
}
