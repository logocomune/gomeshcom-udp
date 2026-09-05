package netconsole

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/packetingest"
)

type lifecycleDialer func(context.Context, string, string) (net.Conn, error)

func (dial lifecycleDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return dial(ctx, network, address)
}

type authenticationReadConn struct {
	net.Conn
	reading chan struct{}
	once    sync.Once
}

func (conn *authenticationReadConn) Read(data []byte) (int, error) {
	conn.once.Do(func() { close(conn.reading) })
	return conn.Conn.Read(data)
}

func TestRunCancelsBlockedAuthentication(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	reading := make(chan struct{})
	cfg := testBridgeConfig()
	cfg.AuthTimeout = time.Minute
	bridge, err := NewBridge(Options{
		Config:    cfg,
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
		Dialer:    &singleDialer{conn: &authenticationReadConn{Conn: client, reading: reading}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bridge.Run(ctx) }()
	select {
	case <-reading:
	case <-time.After(time.Second):
		peer.Close()
		t.Fatal("authentication did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		peer.Close()
		<-done
		t.Fatal("cancellation did not interrupt authentication")
	}
}

type lifecycleConn struct {
	net.Conn
	read func([]byte) (int, error)
}

func (conn lifecycleConn) Read(data []byte) (int, error)    { return conn.read(data) }
func (conn lifecycleConn) Write(data []byte) (int, error)   { return len(data), nil }
func (conn lifecycleConn) Close() error                     { return nil }
func (conn lifecycleConn) SetDeadline(time.Time) error      { return nil }
func (conn lifecycleConn) SetWriteDeadline(time.Time) error { return nil }

func TestReconnectResetsOnlyAfterStableConnectedSession(t *testing.T) {
	tests := []struct {
		name              string
		greeting          string
		connectedDuration time.Duration
		dialFailure       bool
		wantDelay         time.Duration
	}{
		{name: "slow failed dial", dialFailure: true, wantDelay: 2 * time.Millisecond},
		{name: "slow failed authentication", greeting: "FAIL\n", wantDelay: 2 * time.Millisecond},
		{name: "slow authentication short session", greeting: "OK\n", connectedDuration: time.Second - time.Nanosecond, wantDelay: 2 * time.Millisecond},
		{name: "stable session boundary", greeting: "OK\n", connectedDuration: time.Second, wantDelay: time.Millisecond},
		{name: "long stable session", greeting: "OK\n", connectedDuration: 2 * time.Second, wantDelay: time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			attempts := 0
			dialer := lifecycleDialer(func(context.Context, string, string) (net.Conn, error) {
				attempts++
				if attempts == 1 || tt.dialFailure {
					now = now.Add(2 * time.Second)
					return nil, errors.New("dial timeout")
				}
				greeted := false
				return lifecycleConn{read: func(data []byte) (int, error) {
					if !greeted {
						greeted = true
						now = now.Add(2 * time.Second)
						return copy(data, tt.greeting), nil
					}
					now = now.Add(tt.connectedDuration)
					return 0, io.EOF
				}}, nil
			})
			bridge, err := NewBridge(Options{Config: testBridgeConfig(), Dialer: dialer, Processor: packetingest.NewProcessor(packetingest.Dependencies{})})
			if err != nil {
				t.Fatal(err)
			}
			bridge.now = func() time.Time { return now }
			bridge.jitter = func(delay time.Duration) time.Duration { return delay }
			var delays []time.Duration
			bridge.wait = func(_ context.Context, delay time.Duration) bool {
				delays = append(delays, delay)
				return len(delays) < 2
			}
			if err := bridge.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if want := []time.Duration{time.Millisecond, tt.wantDelay}; !reflect.DeepEqual(delays, want) {
				t.Fatalf("retry delays = %v, want %v", delays, want)
			}
		})
	}
}
