package consolecodec

import (
	"bytes"
	"errors"
	"testing"
)

func TestDecoderExtractsConsoleRecordsAcrossArbitraryChunks(t *testing.T) {
	input := []byte("banner\r\n[EXT] Out: {\"type\":\"msg\",\"msg\":\"a } b\"} Len: 42\r\n" +
		"[00:00:00] [EXT] Tele-Out: {\"type\":\"pos\"}\n")
	want := [][]byte{
		[]byte(`{"type":"msg","msg":"a } b"}`),
		[]byte(`{"type":"pos"}`),
	}

	for split := 0; split <= len(input); split++ {
		decoder, err := NewDecoder(1024)
		if err != nil {
			t.Fatalf("NewDecoder() error = %v", err)
		}
		first := decoder.Feed(input[:split])
		second := decoder.Feed(input[split:])
		got := append(first.Payloads, second.Payloads...)
		if len(first.Errors)+len(second.Errors) != 0 || !equalPayloads(got, want) {
			t.Fatalf("split %d: payloads = %q, errors = %v %v", split, got, first.Errors, second.Errors)
		}
	}
}

func TestDecoderRejectsOverlongRecordAndRecovers(t *testing.T) {
	decoder, err := NewDecoder(32)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	result := decoder.Feed(append(bytes.Repeat([]byte{'x'}, 33), []byte("\n[EXT] Out: {\"ok\":true}\n")...))
	if len(result.Errors) != 1 || !errors.Is(result.Errors[0], ErrRecordTooLong) {
		t.Fatalf("errors = %v, want ErrRecordTooLong", result.Errors)
	}
	if len(result.Payloads) != 1 || string(result.Payloads[0]) != `{"ok":true}` {
		t.Fatalf("payloads = %q", result.Payloads)
	}
}

func TestNewDecoderRejectsInvalidLimit(t *testing.T) {
	if _, err := NewDecoder(0); !errors.Is(err, ErrInvalidRecordLimit) {
		t.Fatalf("NewDecoder() error = %v, want ErrInvalidRecordLimit", err)
	}
}

func FuzzDecoder(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("[EXT] Out: {\"type\":\"msg\"}\r\n"),
		[]byte("[EXT] Tele-Out: {\"type\":\"pos\"}\n"),
		bytes.Repeat([]byte{'x'}, 65),
		{0, '\n', 0xff},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		decoder, err := NewDecoder(64)
		if err != nil {
			t.Fatalf("NewDecoder() error = %v", err)
		}
		result := decoder.Feed(input)
		result = mergeDecodeResults(result, decoder.Flush())
		if decoder.BufferedBytes() > 64 {
			t.Fatalf("buffered bytes = %d", decoder.BufferedBytes())
		}
		for _, payload := range result.Payloads {
			if len(payload) > 64 {
				t.Fatalf("payload length = %d", len(payload))
			}
		}
	})
}

func equalPayloads(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !bytes.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}

func mergeDecodeResults(first, second DecodeResult) DecodeResult {
	first.Payloads = append(first.Payloads, second.Payloads...)
	first.Errors = append(first.Errors, second.Errors...)
	return first
}
