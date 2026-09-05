package netconsole

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/consolecodec"
	"github.com/logocomune/gomeshcom-client/internal/packetingest"
	"github.com/logocomune/gomeshcom-client/internal/transport"
)

const enablePacketOutputCommand = "--setinfo on\r\n"

var (
	ErrDisconnected   = fmt.Errorf("netconsole transport is disconnected: %w", transport.ErrUnavailable)
	ErrAlreadyRunning = errors.New("netconsole transport is already running")
)

type Config struct {
	Address          string
	Password         string
	ConnectTimeout   time.Duration
	AuthTimeout      time.Duration
	WriteTimeout     time.Duration
	ReconnectInitial time.Duration
	ReconnectMax     time.Duration
	StableResetAfter time.Duration
	MaxAuthLineBytes int
	MaxRecordBytes   int
}

type Forwarder interface {
	Forward([]byte)
}

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Options struct {
	Config    Config
	Dialer    Dialer
	Processor *packetingest.Processor
	Forwarder Forwarder
	Identity  consolecodec.Identity
	DisableTX bool
}

type Bridge struct {
	config    Config
	dialer    Dialer
	processor *packetingest.Processor
	forwarder Forwarder
	encoder   *consolecodec.Encoder
	disableTX bool

	running atomic.Bool

	statusMu sync.RWMutex
	status   transport.Status

	sessionMu sync.RWMutex
	session   *activeSession

	now    func() time.Time
	wait   func(context.Context, time.Duration) bool
	jitter func(time.Duration) time.Duration
}

func NewBridge(options Options) (*Bridge, error) {
	if err := validateBridgeConfig(options.Config); err != nil {
		return nil, err
	}
	if options.Processor == nil {
		return nil, errors.New("netconsole packet processor is required")
	}
	dialer := options.Dialer
	if dialer == nil {
		dialer = &net.Dialer{
			Timeout:   options.Config.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}
	}
	return &Bridge{
		config:    options.Config,
		dialer:    dialer,
		processor: options.Processor,
		forwarder: normalizeForwarder(options.Forwarder),
		encoder:   consolecodec.NewEncoder(options.Identity),
		disableTX: options.DisableTX,
		status: transport.Status{
			Mode:     "netconsole",
			State:    transport.StateDisconnected,
			Endpoint: options.Config.Address,
		},
		now:    time.Now,
		wait:   waitForRetry,
		jitter: jitterDelay,
	}, nil
}

func validateBridgeConfig(config Config) error {
	if config.Address == "" {
		return errors.New("netconsole address is required")
	}
	if config.ConnectTimeout <= 0 || config.AuthTimeout <= 0 || config.WriteTimeout <= 0 {
		return errors.New("netconsole timeouts must be greater than zero")
	}
	if config.ReconnectInitial <= 0 || config.ReconnectMax <= 0 || config.ReconnectInitial > config.ReconnectMax {
		return errors.New("invalid netconsole reconnect delays")
	}
	if config.StableResetAfter <= 0 {
		return errors.New("netconsole stable reset duration must be greater than zero")
	}
	if config.MaxAuthLineBytes <= 0 || config.MaxRecordBytes <= 0 {
		return errors.New("netconsole buffer limits must be greater than zero")
	}
	return nil
}

func normalizeForwarder(forwarder Forwarder) Forwarder {
	if forwarder == nil {
		return nil
	}
	value := reflect.ValueOf(forwarder)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return forwarder
}

func (b *Bridge) Run(ctx context.Context) error {
	if !b.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	defer func() {
		b.running.Store(false)
		b.setStopped()
	}()

	delay := b.config.ReconnectInitial
	var retryCount uint64
	for {
		if ctx.Err() != nil {
			return nil
		}
		slog.Info("netconsole connection attempt", "address", b.config.Address, "attempt", retryCount+1)
		b.setConnecting(retryCount)
		connectedDuration, err := b.runSession(ctx)
		if ctx.Err() != nil {
			return nil
		}

		retryCount++
		b.setDegraded(err, retryCount)
		if connectedDuration >= b.config.StableResetAfter {
			delay = b.config.ReconnectInitial
		}
		retryDelay := b.jitter(delay)
		slog.Warn("netconsole connection lost; retrying", "address", b.config.Address, "error", err, "retry_in", retryDelay)
		if !b.wait(ctx, retryDelay) {
			return nil
		}
		delay = nextDelay(delay, b.config.ReconnectMax)
	}
}

func (b *Bridge) SendText(ctx context.Context, destination, message string, maxLength int) error {
	command, err := b.encoder.Encode(consolecodec.TextCommand{
		Destination:     destination,
		Message:         message,
		MaxMessageRunes: maxLength,
	})
	if err != nil {
		return err
	}
	if b.disableTX {
		slog.Warn("netconsole TX disabled (dry-run)", "destination", destination)
		return nil
	}
	session := b.connectedSession()
	if session == nil {
		return ErrDisconnected
	}
	if err := session.write(ctx, command, b.config.WriteTimeout); err != nil {
		session.fail(err)
		b.setDegraded(err, b.Status().RetryCount)
		return fmt.Errorf("netconsole TX: %w", err)
	}
	return nil
}

func (b *Bridge) Status() transport.Status {
	b.statusMu.RLock()
	defer b.statusMu.RUnlock()
	return b.status
}

func (b *Bridge) TransportStatus() transport.Status {
	return b.Status()
}

func (b *Bridge) runSession(ctx context.Context) (connectedDuration time.Duration, sessionError error) {
	conn, err := b.dialer.DialContext(ctx, "tcp", b.config.Address)
	if err != nil {
		return 0, fmt.Errorf("connect netconsole: %w", err)
	}
	session := newActiveSession(conn)
	defer session.close()

	stopWatcher := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		select {
		case <-ctx.Done():
			session.close()
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		session.close()
		watcher.Wait()
	}()

	reader := bufio.NewReader(conn)
	if err := authenticate(conn, reader, b.config.Password, b.config.AuthTimeout, b.config.MaxAuthLineBytes); err != nil {
		return 0, err
	}
	decoder, err := consolecodec.NewDecoder(b.config.MaxRecordBytes)
	if err != nil {
		return 0, err
	}

	b.installSession(session)
	defer b.removeSession(session)

	if err := session.write(ctx, []byte(enablePacketOutputCommand), b.config.WriteTimeout); err != nil {
		return 0, fmt.Errorf("enable netconsole packet output: %w", err)
	}
	connectedAt := b.now()
	defer func() {
		connectedDuration = b.now().Sub(connectedAt)
	}()
	b.setConnected()
	slog.Info("netconsole connected", "address", b.config.Address)

	buffer := make([]byte, 4096)
	for {
		bytesRead, readErr := reader.Read(buffer)
		if bytesRead > 0 {
			b.processBytes(decoder, buffer[:bytesRead])
		}
		if failure := session.failureError(); failure != nil {
			return 0, failure
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if errors.Is(readErr, io.EOF) {
				return 0, io.EOF
			}
			return 0, fmt.Errorf("read netconsole: %w", readErr)
		}
	}
}

func (b *Bridge) processBytes(decoder *consolecodec.Decoder, data []byte) {
	result := decoder.Feed(data)
	for _, decodeError := range result.Errors {
		slog.Warn("netconsole record ignored", "address", b.config.Address, "error", decodeError)
	}
	for _, payload := range result.Payloads {
		if b.forwarder != nil {
			b.forwarder.Forward(payload)
		}
		_ = b.processor.Process(packetingest.Source{
			Transport: "netconsole",
			Endpoint:  b.config.Address,
		}, payload)
	}
}

func (b *Bridge) connectedSession() *activeSession {
	if b.Status().State != transport.StateConnected {
		return nil
	}
	b.sessionMu.RLock()
	defer b.sessionMu.RUnlock()
	return b.session
}

func (b *Bridge) installSession(session *activeSession) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	b.session = session
}

func (b *Bridge) removeSession(session *activeSession) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	if b.session == session {
		b.session = nil
	}
}

func (b *Bridge) setConnecting(retryCount uint64) {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.State = transport.StateConnecting
	b.status.RetryCount = retryCount
	b.status.ConnectedAt = nil
}

func (b *Bridge) setConnected() {
	now := b.now().UTC()
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.State = transport.StateConnected
	b.status.LastError = ""
	b.status.LastErrorAt = nil
	b.status.ConnectedAt = &now
}

func (b *Bridge) setDegraded(err error, retryCount uint64) {
	now := b.now().UTC()
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.State = transport.StateDegraded
	b.status.RetryCount = retryCount
	b.status.ConnectedAt = nil
	if err != nil {
		b.status.LastError = err.Error()
		b.status.LastErrorAt = &now
	}
}

func (b *Bridge) setStopped() {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	b.status.State = transport.StateStopped
	b.status.ConnectedAt = nil
}

type activeSession struct {
	conn net.Conn

	writeMu   sync.Mutex
	failMu    sync.Mutex
	failure   error
	closeOnce sync.Once
}

func newActiveSession(conn net.Conn) *activeSession {
	return &activeSession{conn: conn}
}

func (s *activeSession) write(ctx context.Context, data []byte, timeout time.Duration) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if failure := s.failureError(); failure != nil {
		return failure
	}

	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer func() {
		_ = s.conn.SetWriteDeadline(time.Time{})
	}()
	if err := writeAll(s.conn, data); err != nil {
		return err
	}
	return nil
}

func (s *activeSession) fail(err error) {
	s.failMu.Lock()
	if s.failure == nil {
		s.failure = err
	}
	s.failMu.Unlock()
	s.close()
}

func (s *activeSession) failureError() error {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	return s.failure
}

func (s *activeSession) close() {
	s.closeOnce.Do(func() {
		_ = s.conn.Close()
	})
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitterDelay(delay time.Duration) time.Duration {
	if delay <= 1 {
		return delay
	}
	return delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
}

func nextDelay(current, maximum time.Duration) time.Duration {
	if current >= maximum || current > maximum/2 {
		return maximum
	}
	return current * 2
}
