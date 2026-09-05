package netconsole

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var (
	ErrAuthLineTooLong      = errors.New("netconsole authentication line exceeds configured limit")
	ErrMalformedChallenge   = errors.New("malformed netconsole challenge")
	ErrAuthenticationFailed = errors.New("netconsole authentication rejected")
	ErrUnexpectedAuthLine   = errors.New("unexpected netconsole authentication line")
)

const noncePrefix = "NONCE: "

func authenticate(conn net.Conn, reader *bufio.Reader, password string, timeout time.Duration, maxLineBytes int) error {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set authentication deadline: %w", err)
	}
	defer func() {
		_ = conn.SetDeadline(time.Time{})
	}()

	firstLine, err := readBoundedLine(reader, maxLineBytes)
	if err != nil {
		return fmt.Errorf("read authentication greeting: %w", err)
	}
	if firstLine == "OK" {
		return nil
	}
	if !strings.HasPrefix(firstLine, noncePrefix) {
		return ErrUnexpectedAuthLine
	}

	nonceHex := strings.TrimPrefix(firstLine, noncePrefix)
	response, err := hmacResponse(password, nonceHex)
	if err != nil {
		return err
	}
	if err := writeAll(conn, []byte(response+"\r\n")); err != nil {
		return fmt.Errorf("write authentication response: %w", err)
	}

	result, err := readBoundedLine(reader, maxLineBytes)
	if err != nil {
		return fmt.Errorf("read authentication result: %w", err)
	}
	switch result {
	case "OK":
		return nil
	case "FAIL":
		return ErrAuthenticationFailed
	default:
		return ErrUnexpectedAuthLine
	}
}

func readBoundedLine(reader *bufio.Reader, maxBytes int) (string, error) {
	line := make([]byte, 0, min(maxBytes, 128))
	for {
		value, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return "", io.ErrUnexpectedEOF
			}
			return "", err
		}
		if len(line) >= maxBytes {
			return "", ErrAuthLineTooLong
		}
		line = append(line, value)
		if value == '\n' {
			break
		}
	}
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return string(line), nil
}

func hmacResponse(password, nonceHex string) (string, error) {
	if len(nonceHex) != 32 {
		return "", fmt.Errorf("%w: nonce must contain 32 hexadecimal characters", ErrMalformedChallenge)
	}
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil || len(nonce) != 16 {
		return "", fmt.Errorf("%w: invalid nonce", ErrMalformedChallenge)
	}
	mac := hmac.New(sha256.New, []byte(password))
	if _, err := mac.Write(nonce); err != nil {
		return "", fmt.Errorf("hash nonce: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
