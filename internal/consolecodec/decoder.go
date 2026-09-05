package consolecodec

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	ErrInvalidRecordLimit = errors.New("console record limit must be greater than zero")
	ErrRecordTooLong      = errors.New("console record exceeds configured limit")
	ErrMalformedExtRecord = errors.New("malformed console ExtUDP record")
)

var extMarkers = [][]byte{
	[]byte("[EXT] Tele-Out: "),
	[]byte("[EXT] Out: "),
}

type DecodeResult struct {
	Payloads [][]byte
	Errors   []error
}

type Decoder struct {
	maxRecordBytes int
	record         []byte
	discarding     bool
}

func NewDecoder(maxRecordBytes int) (*Decoder, error) {
	if maxRecordBytes <= 0 {
		return nil, ErrInvalidRecordLimit
	}
	return &Decoder{
		maxRecordBytes: maxRecordBytes,
		record:         make([]byte, 0, min(maxRecordBytes, 4096)),
	}, nil
}

func (d *Decoder) Feed(data []byte) DecodeResult {
	var result DecodeResult
	for _, value := range data {
		d.consumeByte(value, &result)
	}
	return result
}

func (d *Decoder) Flush() DecodeResult {
	if d.discarding {
		d.discarding = false
		d.record = d.record[:0]
		return DecodeResult{}
	}
	if len(d.record) == 0 {
		return DecodeResult{}
	}
	result := DecodeResult{}
	d.finishRecord(&result)
	return result
}

func (d *Decoder) BufferedBytes() int {
	return len(d.record)
}

func (d *Decoder) consumeByte(value byte, result *DecodeResult) {
	if d.discarding {
		if value == '\n' {
			d.discarding = false
		}
		return
	}
	if value == '\n' {
		d.finishRecord(result)
		return
	}
	if len(d.record) >= d.maxRecordBytes {
		d.record = d.record[:0]
		d.discarding = true
		result.Errors = append(result.Errors, ErrRecordTooLong)
		return
	}
	d.record = append(d.record, value)
}

func (d *Decoder) finishRecord(result *DecodeResult) {
	record := d.record
	d.record = d.record[:0]
	record = bytes.TrimSuffix(record, []byte{'\r'})

	payload, supported, err := extractExtPayload(record)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	if supported {
		result.Payloads = append(result.Payloads, payload)
	}
}

func extractExtPayload(record []byte) ([]byte, bool, error) {
	record = trimCapturePrefix(bytes.TrimSpace(record))
	var body []byte
	for _, marker := range extMarkers {
		if bytes.HasPrefix(record, marker) {
			body = record[len(marker):]
			break
		}
	}
	if body == nil {
		return nil, false, nil
	}

	start := bytes.IndexByte(body, '{')
	if start < 0 {
		return nil, true, fmt.Errorf("%w: missing JSON object", ErrMalformedExtRecord)
	}
	end, ok := matchingObjectEnd(body[start:])
	if !ok {
		return nil, true, fmt.Errorf("%w: incomplete JSON object", ErrMalformedExtRecord)
	}
	payload := append([]byte(nil), body[start:start+end]...)
	return payload, true, nil
}

func trimCapturePrefix(record []byte) []byte {
	if len(record) < len("[00:00:00]") || record[0] != '[' {
		return record
	}
	closeIndex := bytes.IndexByte(record, ']')
	if closeIndex != len("[00:00:00]")-1 || !isCaptureTime(record[1:closeIndex]) {
		return record
	}
	return bytes.TrimSpace(record[closeIndex+1:])
}

func isCaptureTime(value []byte) bool {
	if len(value) != len("00:00:00") || value[2] != ':' || value[5] != ':' {
		return false
	}
	for index, character := range value {
		if index == 2 || index == 5 {
			continue
		}
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func matchingObjectEnd(data []byte) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for index, character := range data {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch character {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}
