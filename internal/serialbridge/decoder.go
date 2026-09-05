package serialbridge

import "github.com/logocomune/gomeshcom-client/internal/consolecodec"

var (
	ErrInvalidRecordLimit = consolecodec.ErrInvalidRecordLimit
	ErrRecordTooLong      = consolecodec.ErrRecordTooLong
	ErrMalformedExtRecord = consolecodec.ErrMalformedExtRecord
)

type DecodeResult = consolecodec.DecodeResult
type Decoder = consolecodec.Decoder

func NewDecoder(maxRecordBytes int) (*Decoder, error) {
	return consolecodec.NewDecoder(maxRecordBytes)
}
