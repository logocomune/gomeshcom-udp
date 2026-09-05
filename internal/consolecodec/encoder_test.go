package consolecodec

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type fixedIdentity string

func (identity fixedIdentity) Current() string {
	return string(identity)
}

func TestEncoderFormatsConsoleCommands(t *testing.T) {
	encoder := NewEncoder(fixedIdentity("QQ1LOCAL-1"))
	tests := []struct {
		name        string
		destination string
		message     string
		want        string
	}{
		{name: "broadcast", destination: "*", message: "Hello", want: "::Hello\r\n"},
		{name: "channel", destination: "42", message: "Hello", want: "::{42}Hello\r\n"},
		{name: "direct", destination: "qq1peer-2", message: "Hello", want: "::{QQ1PEER-2}Hello\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encoder.Encode(TextCommand{
				Destination:     tt.destination,
				Message:         tt.message,
				MaxMessageRunes: 149,
			})
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEncoderRejectsUnsafeCommands(t *testing.T) {
	encoder := NewEncoder(fixedIdentity("QQ1LOCAL-1"))
	tests := []struct {
		name    string
		command TextCommand
		wantErr error
	}{
		{name: "command injection", command: TextCommand{Destination: "*", Message: "one\r\ntwo", MaxMessageRunes: 149}, wantErr: ErrCommandInjection},
		{name: "invalid channel", command: TextCommand{Destination: "0", Message: "text", MaxMessageRunes: 149}, wantErr: ErrInvalidDestination},
		{name: "self direct message", command: TextCommand{Destination: "qq1local-1", Message: "text", MaxMessageRunes: 149}, wantErr: ErrSelfDirectMessage},
		{name: "firmware byte limit", command: TextCommand{Destination: "*", Message: strings.Repeat("x", 161), MaxMessageRunes: 200}, wantErr: ErrPayloadTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := encoder.Encode(tt.command); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Encode() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func FuzzEncoder(f *testing.F) {
	for _, seed := range []struct {
		destination string
		message     string
	}{
		{destination: "*", message: "Hello"},
		{destination: "42", message: "Channel"},
		{destination: "QQ1PEER-2", message: "Direct"},
		{destination: "*", message: "line\nbreak"},
	} {
		f.Add(seed.destination, seed.message)
	}
	f.Fuzz(func(t *testing.T, destination, message string) {
		encoded, err := NewEncoder(fixedIdentity("QQ1LOCAL-1")).Encode(TextCommand{
			Destination:     destination,
			Message:         message,
			MaxMessageRunes: 149,
		})
		if err != nil {
			return
		}
		if !bytes.HasPrefix(encoded, []byte("::")) || !bytes.HasSuffix(encoded, []byte("\r\n")) {
			t.Fatalf("encoded command = %q", encoded)
		}
		if bytes.ContainsAny(encoded[:len(encoded)-2], "\r\n\x00") {
			t.Fatalf("encoded command contains separator: %q", encoded)
		}
	})
}
