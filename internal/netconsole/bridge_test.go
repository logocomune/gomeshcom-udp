package netconsole

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/events"
	"github.com/logocomune/gomeshcom-client/internal/packetingest"
	"github.com/logocomune/gomeshcom-client/internal/transport"
)

type singleDialer struct {
	conn net.Conn
	once sync.Once
}

type sequenceDialer struct {
	mu          sync.Mutex
	connections []net.Conn
	dials       int
}

func (d *sequenceDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dials >= len(d.connections) {
		return nil, io.ErrClosedPipe
	}
	connection := d.connections[d.dials]
	d.dials++
	return connection, nil
}

func (d *sequenceDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

func (d *singleDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	var conn net.Conn
	d.once.Do(func() {
		conn = d.conn
	})
	if conn == nil {
		return nil, io.ErrClosedPipe
	}
	return conn, nil
}

type recordingForwarder struct {
	mu       sync.Mutex
	payloads [][]byte
}

func (f *recordingForwarder) Forward(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payloads = append(f.payloads, append([]byte(nil), payload...))
}

func (f *recordingForwarder) first() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.payloads) == 0 {
		return nil
	}
	return append([]byte(nil), f.payloads[0]...)
}

func TestBridgeReceivesSendsReportsStatusAndStops(t *testing.T) {
	client, server := net.Pipe()
	dialer := &singleDialer{conn: client}
	bus := events.NewBus()
	eventContext, cancelEvents := context.WithCancel(context.Background())
	defer cancelEvents()
	subscriber := bus.Subscribe(eventContext)
	forwarder := &recordingForwarder{}

	bridge, err := NewBridge(Options{
		Config: Config{
			Address:          "meshcom.local:2323",
			ConnectTimeout:   time.Second,
			AuthTimeout:      time.Second,
			WriteTimeout:     time.Second,
			ReconnectInitial: time.Millisecond,
			ReconnectMax:     time.Millisecond,
			StableResetAfter: time.Second,
			MaxAuthLineBytes: 128,
			MaxRecordBytes:   65536,
		},
		Dialer:    dialer,
		Processor: packetingest.NewProcessor(packetingest.Dependencies{Bus: bus}),
		Forwarder: forwarder,
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}

	serverLines := make(chan string, 2)
	go func() {
		reader := bufio.NewReader(server)
		_, _ = io.WriteString(server, "OK\r\nMeshCom Console\r\n")
		for range 2 {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			serverLines <- line
		}
	}()

	runContext, cancelRun := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() {
		runResult <- bridge.Run(runContext)
	}()

	waitForState(t, bridge, transport.StateConnected)
	if line := <-serverLines; line != enablePacketOutputCommand {
		t.Fatalf("startup command = %q", line)
	}
	if err := bridge.SendText(context.Background(), "qq1peer-2", "Hello", 149); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if line := <-serverLines; line != "::{QQ1PEER-2}Hello\r\n" {
		t.Fatalf("TX command = %q", line)
	}

	payload := `{"src_type":"lora","type":"msg","src":"QQ1PEER-2","dst":"*","msg":"Hello"}`
	_, _ = io.WriteString(server, "[EXT] Out: "+payload+" Len: 80\r\n")
	select {
	case event := <-subscriber:
		data := event.Data.(map[string]any)
		if data["transport"] != "netconsole" || data["endpoint"] != "meshcom.local:2323" {
			t.Fatalf("event metadata = %+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for packet event")
	}
	if got := forwarder.first(); !bytes.Equal(got, []byte(payload)) {
		t.Fatalf("forwarded = %q, want %q", got, payload)
	}

	cancelRun()
	_ = server.Close()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop")
	}
	if bridge.Status().State != transport.StateStopped {
		t.Fatalf("state = %q, want stopped", bridge.Status().State)
	}
}

func TestSendTextReturnsUnavailableBeforeConnection(t *testing.T) {
	bridge, err := NewBridge(Options{
		Config: Config{
			Address:          "meshcom.local:2323",
			ConnectTimeout:   time.Second,
			AuthTimeout:      time.Second,
			WriteTimeout:     time.Second,
			ReconnectInitial: time.Second,
			ReconnectMax:     time.Second,
			StableResetAfter: time.Second,
			MaxAuthLineBytes: 128,
			MaxRecordBytes:   65536,
		},
		Dialer:    &singleDialer{},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	if err := bridge.SendText(context.Background(), "*", "Hello", 149); !errors.Is(err, transport.ErrUnavailable) {
		t.Fatalf("SendText() error = %v, want transport unavailable", err)
	}
}

func TestBridgeReconnectsAfterAuthenticationFailureAndSessionLoss(t *testing.T) {
	tests := []struct {
		name          string
		firstServer   func(net.Conn)
		wantErrorText string
	}{
		{
			name: "authentication failure",
			firstServer: func(connection net.Conn) {
				defer connection.Close()
				_, _ = io.WriteString(connection, "NONCE: 6fe0be2e326e121a4f8157426524a521\r\n")
				_, _ = bufio.NewReader(connection).ReadString('\n')
				_, _ = io.WriteString(connection, "FAIL\r\n")
			},
			wantErrorText: ErrAuthenticationFailed.Error(),
		},
		{
			name: "established session loss",
			firstServer: func(connection net.Conn) {
				defer connection.Close()
				_, _ = io.WriteString(connection, "OK\r\n")
				_, _ = bufio.NewReader(connection).ReadString('\n')
			},
			wantErrorText: io.EOF.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstClient, firstServer := net.Pipe()
			secondClient, secondServer := net.Pipe()
			dialer := &sequenceDialer{connections: []net.Conn{firstClient, secondClient}}
			bridge, err := NewBridge(Options{
				Config:    testBridgeConfig(),
				Dialer:    dialer,
				Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
			})
			if err != nil {
				t.Fatalf("NewBridge() error = %v", err)
			}
			bridge.jitter = func(delay time.Duration) time.Duration { return delay }
			retryObserved := make(chan transport.Status, 1)
			bridge.wait = func(ctx context.Context, _ time.Duration) bool {
				retryObserved <- bridge.Status()
				return ctx.Err() == nil
			}

			go tt.firstServer(firstServer)
			secondReady := make(chan struct{})
			secondRelease := make(chan struct{})
			defer close(secondRelease)
			go func() {
				defer secondServer.Close()
				_, _ = io.WriteString(secondServer, "OK\r\n")
				line, readErr := bufio.NewReader(secondServer).ReadString('\n')
				if readErr == nil && line == enablePacketOutputCommand {
					close(secondReady)
					<-secondRelease
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			runResult := make(chan error, 1)
			go func() { runResult <- bridge.Run(ctx) }()

			select {
			case status := <-retryObserved:
				if status.State != transport.StateDegraded ||
					!strings.Contains(status.LastError, tt.wantErrorText) {
					t.Fatalf("retry status = %+v, want error containing %q", status, tt.wantErrorText)
				}
			case <-time.After(time.Second):
				t.Fatal("retry not observed")
			}
			select {
			case <-secondReady:
			case <-time.After(time.Second):
				t.Fatal("second session did not connect")
			}
			waitForState(t, bridge, transport.StateConnected)
			if dialer.count() != 2 {
				t.Fatalf("dial count = %d, want 2", dialer.count())
			}

			cancel()
			select {
			case err := <-runResult:
				if err != nil {
					t.Fatalf("Run() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Run() did not stop")
			}
		})
	}
}

func TestBridgeLogsNeverContainAuthenticationMaterial(t *testing.T) {
	const (
		password = "pw-never-log"
		nonce    = "6fe0be2e326e121a4f8157426524a521"
	)
	client, server := net.Pipe()
	defer server.Close()
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	bridgeConfig := testBridgeConfig()
	bridgeConfig.Password = password
	bridge, err := NewBridge(Options{
		Config:    bridgeConfig,
		Dialer:    &singleDialer{conn: client},
		Processor: packetingest.NewProcessor(packetingest.Dependencies{}),
	})
	if err != nil {
		t.Fatalf("NewBridge() error = %v", err)
	}
	bridge.wait = func(context.Context, time.Duration) bool { return false }

	digest, err := hmacResponse(password, nonce)
	if err != nil {
		t.Fatalf("hmacResponse() error = %v", err)
	}
	go func() {
		_, _ = io.WriteString(server, noncePrefix+nonce+"\r\n")
		_, _ = bufio.NewReader(server).ReadString('\n')
		_, _ = io.WriteString(server, "FAIL\r\n")
	}()

	if err := bridge.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for name, secret := range map[string]string{
		"password": password,
		"nonce":    nonce,
		"digest":   digest,
	} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("daemon log contains %s", name)
		}
	}
}

func waitForState(t *testing.T, bridge *Bridge, state transport.State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if bridge.Status().State == state {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %q, want %q", bridge.Status().State, state)
}
