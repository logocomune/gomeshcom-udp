package netconsole

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

func TestAuthenticate(t *testing.T) {
	const nonce = "6fe0be2e326e121a4f8157426524a521"
	tests := []struct {
		name         string
		greeting     string
		result       string
		password     string
		wantErr      error
		wantResponse bool
	}{
		{name: "open access", greeting: "OK\r\n"},
		{name: "authenticated", greeting: "NONCE: " + nonce + "\r\n", result: "OK\r\n", password: "pippo", wantResponse: true},
		{name: "uppercase nonce", greeting: "NONCE: " + strings.ToUpper(nonce) + "\n", result: "OK\n", password: "pippo", wantResponse: true},
		{name: "rejected", greeting: "NONCE: " + nonce + "\r\n", result: "FAIL\r\n", password: "wrong", wantErr: ErrAuthenticationFailed, wantResponse: true},
		{name: "empty password rejected", greeting: "NONCE: " + nonce + "\r\n", result: "FAIL\r\n", wantErr: ErrAuthenticationFailed, wantResponse: true},
		{name: "unknown greeting", greeting: "WELCOME\r\n", wantErr: ErrUnexpectedAuthLine},
		{name: "short nonce", greeting: "NONCE: 1234\r\n", wantErr: ErrMalformedChallenge},
		{name: "invalid nonce", greeting: "NONCE: zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz\r\n", wantErr: ErrMalformedChallenge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			serverResult := make(chan string, 1)
			go func() {
				_, _ = io.WriteString(server, tt.greeting)
				if !tt.wantResponse {
					serverResult <- ""
					return
				}
				line, _ := bufio.NewReader(server).ReadString('\n')
				serverResult <- line
				_, _ = io.WriteString(server, tt.result)
			}()

			err := authenticate(client, bufio.NewReader(client), tt.password, time.Second, 128)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("authenticate() error = %v", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("authenticate() error = %v, want %v", err, tt.wantErr)
			}

			response := <-serverResult
			if tt.wantResponse {
				nonceHex := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(tt.greeting), noncePrefix), "\r")
				decoded, decodeErr := hex.DecodeString(nonceHex)
				if decodeErr != nil {
					t.Fatalf("decode test nonce: %v", decodeErr)
				}
				mac := hmac.New(sha256.New, []byte(tt.password))
				_, _ = mac.Write(decoded)
				want := hex.EncodeToString(mac.Sum(nil)) + "\r\n"
				if response != want {
					t.Fatalf("response = %q, want %q", response, want)
				}
			}
		})
	}
}

func TestAuthenticateHandlesFragmentedChallenge(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		for _, fragment := range []string{"NON", "CE: 6fe0be2e", "326e121a4f8157426524a521\r", "\n"} {
			_, _ = io.WriteString(server, fragment)
		}
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = io.WriteString(server, "OK\r\n")
	}()

	if err := authenticate(client, bufio.NewReader(client), "secret", time.Second, 128); err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
}

func TestAuthenticateTimesOutWithoutNewline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _, _ = io.WriteString(server, "OK") }()

	err := authenticate(client, bufio.NewReader(client), "", 10*time.Millisecond, 128)
	var networkError net.Error
	if !errors.As(err, &networkError) || !networkError.Timeout() {
		t.Fatalf("authenticate() error = %v, want timeout", err)
	}
}

func TestAuthenticateClearsDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		_, _ = io.WriteString(server, "OK\r\n")
		time.Sleep(25 * time.Millisecond)
		_, _ = io.WriteString(server, "session data\r\n")
	}()

	reader := bufio.NewReader(client)
	if err := authenticate(client, reader, "", 10*time.Millisecond, 128); err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil || line != "session data\r\n" {
		t.Fatalf("session read = %q, %v", line, err)
	}
}

func TestHMACResponseProperty(t *testing.T) {
	property := func(nonce [16]byte, password string) bool {
		response, err := hmacResponse(password, hex.EncodeToString(nonce[:]))
		if err != nil {
			return false
		}
		mac := hmac.New(sha256.New, []byte(password))
		_, _ = mac.Write(nonce[:])
		want := mac.Sum(nil)
		got, err := hex.DecodeString(response)
		return err == nil && hmac.Equal(got, want)
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatePreservesBufferedSessionBytes(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		_, _ = io.WriteString(server, "OK\r\nMeshCom Console\r\n")
	}()

	reader := bufio.NewReader(client)
	if err := authenticate(client, reader, "", time.Second, 128); err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString() error = %v", err)
	}
	if line != "MeshCom Console\r\n" {
		t.Fatalf("buffered line = %q", line)
	}
}

func TestAuthenticateRejectsOverlongAndIncompleteLines(t *testing.T) {
	tests := []struct {
		name    string
		write   string
		close   bool
		wantErr error
	}{
		{name: "overlong", write: strings.Repeat("x", 17) + "\n", wantErr: ErrAuthLineTooLong},
		{name: "unexpected EOF", write: "OK", close: true, wantErr: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			go func() {
				_, _ = io.WriteString(server, tt.write)
				if tt.close {
					_ = server.Close()
				}
			}()
			err := authenticate(client, bufio.NewReader(client), "", time.Second, 16)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("authenticate() error = %v, want %v", err, tt.wantErr)
			}
			_ = server.Close()
		})
	}
}

func FuzzReadBoundedLine(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("OK\r\n"),
		[]byte("FAIL\n"),
		[]byte("NONCE: 6fe0be2e326e121a4f8157426524a521\r\n"),
		[]byte(strings.Repeat("x", 129) + "\n"),
		{0, '\n'},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		reader := bufio.NewReader(strings.NewReader(string(input)))
		line, err := readBoundedLine(reader, 128)
		if err == nil && len(line) > 127 {
			t.Fatalf("line length = %d", len(line))
		}
	})
}
