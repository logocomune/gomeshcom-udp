package netconsole

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/packetingest"
	"github.com/logocomune/gomeshcom-client/internal/transport"
)

func testBridgeConfig() Config {
	return Config{
		Address:          "meshcom.local:2323",
		ConnectTimeout:   time.Second,
		AuthTimeout:      time.Second,
		WriteTimeout:     time.Second,
		ReconnectInitial: time.Millisecond,
		ReconnectMax:     time.Second,
		StableResetAfter: time.Second,
		MaxAuthLineBytes: 128,
		MaxRecordBytes:   65536,
	}
}

func TestNewBridgeValidatesDependenciesAndConfig(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "address", change: func(config *Config) { config.Address = "" }},
		{name: "connect timeout", change: func(config *Config) { config.ConnectTimeout = 0 }},
		{name: "authentication timeout", change: func(config *Config) { config.AuthTimeout = 0 }},
		{name: "write timeout", change: func(config *Config) { config.WriteTimeout = 0 }},
		{name: "reconnect initial", change: func(config *Config) { config.ReconnectInitial = 0 }},
		{name: "reconnect maximum", change: func(config *Config) { config.ReconnectMax = 0 }},
		{name: "reconnect ordering", change: func(config *Config) { config.ReconnectInitial = 2 * time.Second }},
		{name: "stable reset", change: func(config *Config) { config.StableResetAfter = 0 }},
		{name: "authentication limit", change: func(config *Config) { config.MaxAuthLineBytes = 0 }},
		{name: "record limit", change: func(config *Config) { config.MaxRecordBytes = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := testBridgeConfig()
			tt.change(&config)
			if _, err := NewBridge(Options{
				Config:    config,
				Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
			}); err == nil {
				t.Fatal("NewBridge() error = nil")
			}
		})
	}

	if _, err := NewBridge(Options{Config: testBridgeConfig()}); err == nil {
		t.Fatal("NewBridge() without processor error = nil")
	}
	bridge, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	if bridge.TransportStatus().Mode != "netconsole" {
		t.Fatalf("TransportStatus() = %+v", bridge.TransportStatus())
	}
}

type failingDialer struct {
	err error
}

func (d failingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, d.err
}

func TestRunReportsFailedConnectionAndStopsRetrying(t *testing.T) {
	bridge, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Dialer:    failingDialer{err: errors.New("offline")},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	bridge.jitter = func(delay time.Duration) time.Duration { return delay }
	bridge.wait = func(_ context.Context, delay time.Duration) bool {
		status := bridge.Status()
		if status.State != transport.StateDegraded ||
			status.RetryCount != 1 ||
			status.LastError == "" ||
			delay != bridge.config.ReconnectInitial {
			t.Fatalf("retry status = %+v, delay = %v", status, delay)
		}
		return false
	}

	if err := bridge.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bridge.Status().State != transport.StateStopped {
		t.Fatalf("state = %q, want stopped", bridge.Status().State)
	}
}

func TestRunRejectsConcurrentInvocation(t *testing.T) {
	bridge, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Dialer:    failingDialer{err: errors.New("offline")},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	blocked := make(chan struct{})
	bridge.wait = func(context.Context, time.Duration) bool {
		<-blocked
		return false
	}
	first := make(chan error, 1)
	go func() {
		first <- bridge.Run(context.Background())
	}()
	deadline := time.Now().Add(time.Second)
	for !bridge.running.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := bridge.Run(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Run() error = %v, want ErrAlreadyRunning", err)
	}
	close(blocked)
	if err := <-first; err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
}

func TestSendTextDryRunAndWriteFailure(t *testing.T) {
	dryRun, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Dialer:    failingDialer{},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
		DisableTX: true,
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	if err := dryRun.SendText(context.Background(), "*", "Hello", 149); err != nil {
		t.Fatalf("dry-run SendText() error = %v", err)
	}

	client, server := net.Pipe()
	bridge, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Dialer:    failingDialer{},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	session := newActiveSession(client)
	bridge.installSession(session)
	bridge.setConnected()
	_ = server.Close()

	if err := bridge.SendText(context.Background(), "*", "Hello", 149); err == nil {
		t.Fatal("SendText() error = nil")
	}
	if bridge.Status().State != transport.StateDegraded || session.failureError() == nil {
		t.Fatalf("status = %+v, session failure = %v", bridge.Status(), session.failureError())
	}
}

type limitedConn struct {
	net.Conn
	maxWrite int
}

func (c limitedConn) Write(data []byte) (int, error) {
	if len(data) > c.maxWrite {
		data = data[:c.maxWrite]
	}
	return c.Conn.Write(data)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) {
	return 0, nil
}

type closeCountingConn struct {
	net.Conn
	closeCount atomic.Int32
}

func (connection *closeCountingConn) Close() error {
	connection.closeCount.Add(1)
	return connection.Conn.Close()
}

func TestSessionCompletesPartialWritesAndWriteAllRejectsZero(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	session := newActiveSession(limitedConn{Conn: client, maxWrite: 2})
	want := []byte("abcdef")
	got := make(chan []byte, 1)
	go func() {
		data := make([]byte, len(want))
		_, _ = io.ReadFull(server, data)
		got <- data
	}()

	if err := session.write(context.Background(), want, time.Second); err != nil {
		t.Fatalf("write() error = %v", err)
	}
	if data := <-got; string(data) != string(want) {
		t.Fatalf("written = %q, want %q", data, want)
	}
	session.close()

	if err := writeAll(zeroWriter{}, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeAll() error = %v, want ErrShortWrite", err)
	}
}

func TestSendTextSerializesConcurrentWrites(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	bridge, err := NewBridge(Options{
		Config:    testBridgeConfig(),
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	session := newActiveSession(limitedConn{Conn: client, maxWrite: 1})
	defer session.close()
	bridge.installSession(session)
	bridge.setConnected()

	wantFirst := "::AAAA\r\n::BBBB\r\n"
	wantSecond := "::BBBB\r\n::AAAA\r\n"
	written := make(chan string, 1)
	go func() {
		data := make([]byte, len(wantFirst))
		_, _ = io.ReadFull(server, data)
		written <- string(data)
	}()

	start := make(chan struct{})
	errorsChannel := make(chan error, 2)
	for _, message := range []string{"AAAA", "BBBB"} {
		message := message
		go func() {
			<-start
			errorsChannel <- bridge.SendText(context.Background(), "*", message, 149)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errorsChannel; err != nil {
			t.Fatalf("SendText() error = %v", err)
		}
	}
	if got := <-written; got != wantFirst && got != wantSecond {
		t.Fatalf("serialized writes = %q", got)
	}
}

func TestSessionWriteRejectsCancelledContext(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	session := newActiveSession(client)
	if err := session.write(ctx, []byte("ignored"), time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("write() error = %v, want context.Canceled", err)
	}
}

func TestSessionClosesConnectionOnce(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	connection := &closeCountingConn{Conn: client}
	session := newActiveSession(connection)

	session.close()
	session.close()
	session.fail(errors.New("late failure"))
	if got := connection.closeCount.Load(); got != 1 {
		t.Fatalf("Close() count = %d, want 1", got)
	}
}

func TestRetryHelpers(t *testing.T) {
	if got := nextDelay(time.Second, 8*time.Second); got != 2*time.Second {
		t.Fatalf("nextDelay() = %v", got)
	}
	if got := nextDelay(5*time.Second, 8*time.Second); got != 8*time.Second {
		t.Fatalf("capped nextDelay() = %v", got)
	}
	if got := nextDelay(8*time.Second, 8*time.Second); got != 8*time.Second {
		t.Fatalf("maximum nextDelay() = %v", got)
	}
	if got := jitterDelay(1); got != 1 {
		t.Fatalf("jitterDelay(1) = %v", got)
	}
	for range 20 {
		got := jitterDelay(time.Second)
		if got < 500*time.Millisecond || got > time.Second {
			t.Fatalf("jitterDelay() = %v", got)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitForRetry(ctx, time.Hour) {
		t.Fatal("waitForRetry() = true after cancellation")
	}
	if !waitForRetry(context.Background(), time.Millisecond) {
		t.Fatal("waitForRetry() = false after timer")
	}
}
