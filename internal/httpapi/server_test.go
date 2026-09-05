package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/chatlog"
	"github.com/logocomune/gomeshcom-client/internal/chatstatus"
	"github.com/logocomune/gomeshcom-client/internal/config"
	"github.com/logocomune/gomeshcom-client/internal/dmstats"
	"github.com/logocomune/gomeshcom-client/internal/events"
	"github.com/logocomune/gomeshcom-client/internal/logfmt"
	"github.com/logocomune/gomeshcom-client/internal/meshcom"
	"github.com/logocomune/gomeshcom-client/internal/outbox"
	"github.com/logocomune/gomeshcom-client/internal/positions"
	"github.com/logocomune/gomeshcom-client/internal/receivelog"
	"github.com/logocomune/gomeshcom-client/internal/sendcache"
	"github.com/logocomune/gomeshcom-client/internal/transport"
	"github.com/logocomune/gomeshcom-client/internal/udpbridge"
)

func newHTTPChatLog(t *testing.T, identity staticCallsign) *chatlog.Logger {
	t.Helper()
	return newHTTPChatLogWithDB(t, openHTTPChatLogDB(t), identity)
}

func newHTTPChatLogWithDB(t *testing.T, db *sql.DB, identity staticCallsign) *chatlog.Logger {
	t.Helper()
	return chatlog.NewSQLite(db, identity)
}

func openHTTPChatLogDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gomeshcom.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE chats_dm (id INTEGER PRIMARY KEY AUTOINCREMENT, conversation_id TEXT NOT NULL, msg_id TEXT, sequence_id TEXT, received_at TEXT NOT NULL, src TEXT, src_type TEXT, via TEXT, dst TEXT NOT NULL, msg TEXT NOT NULL, rssi INTEGER, snr INTEGER, direction TEXT, delivery_status TEXT, ack_status TEXT, ack_received_at TEXT, ack_src TEXT, ack_src_type TEXT, ack_rssi INTEGER, ack_snr INTEGER, ack_via TEXT)`,
		`CREATE TABLE chats_public (id INTEGER PRIMARY KEY AUTOINCREMENT, conversation_id TEXT NOT NULL, kind TEXT NOT NULL, channel TEXT, msg_id TEXT, received_at TEXT NOT NULL, src TEXT, src_type TEXT, via TEXT, dst TEXT NOT NULL, msg TEXT NOT NULL, rssi INTEGER, snr INTEGER)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("create chatlog table: %v", err)
		}
	}
	return db
}

func newHTTPReceiveLog(t *testing.T, cfg receivelog.Config) *receivelog.Logger {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gomeshcom.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE receive_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			received_at TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			bytes INTEGER NOT NULL,
			raw TEXT NOT NULL,
			packet_type TEXT,
			parse_error TEXT
		)
	`); err != nil {
		t.Fatalf("create receive_log table: %v", err)
	}
	return receivelog.NewSQLite(cfg, db)
}

func newHTTPPositionsStore(t *testing.T) *positions.Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gomeshcom.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE nodes (
			node_id TEXT PRIMARY KEY, lat REAL, lng REAL, alt INTEGER, hw_id TEXT,
			firstseen TEXT, lastseen TEXT, lastdirectseen TEXT, rssi INTEGER, snr INTEGER, via TEXT
		)
	`); err != nil {
		t.Fatalf("create nodes: %v", err)
	}
	return positions.NewSQLite(db)
}

func openHTTPDMStatsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gomeshcom.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE dm_stats (callsign TEXT PRIMARY KEY, sent INTEGER NOT NULL, ack INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create dm_stats: %v", err)
	}
	return db
}

// staticCallsign is a test-only myCallSource returning a fixed callsign.
// It satisfies chatlog.myCallSource (unexported interface) via structural typing.
type staticCallsign string

func (s staticCallsign) Current() string { return string(s) }

// stubBridge is a fake messageSender for tests.
type stubBridge struct {
	calls  atomic.Int32
	err    error
	onSend func()
}

type staticTransportStatus struct {
	status transport.Status
}

func (s staticTransportStatus) TransportStatus() transport.Status {
	return s.status
}

func (b *stubBridge) SendText(_ context.Context, _, _ string, _ int) error {
	b.calls.Add(1)
	if b.onSend != nil {
		b.onSend()
	}
	return b.err
}

func TestAPIResponsesDisableCaching(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name string
		path string
	}{
		{name: "success", path: "/api/health"},
		{name: "error", path: "/api/events?from=bad"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			assertNoCacheHeaders(t, response.Header())
		})
	}
}

func TestEventStreamDisablesCaching(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	assertNoCacheHeaders(t, response.Header())
}

func TestMutableAppShellResourcesDisableCaching(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	paths := []string{"/", "/index.html", "/service-worker.js", "/manifest.webmanifest", "/unknown-route"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			assertIndexNoCacheHeaders(t, response.Header())
		})
	}
}

func TestImmutableStaticPathsUseLongCache(t *testing.T) {
	header := make(http.Header)
	setCacheHeaders(header, "/_app/immutable/entry/app.js")

	if got := header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable cache", got)
	}
}

func assertIndexNoCacheHeaders(t *testing.T, header http.Header) {
	t.Helper()
	expected := map[string]string{
		"Cache-Control": "no-cache, must-revalidate",
		"Pragma":        "no-cache",
		"Expires":       "0",
	}
	for name, want := range expected {
		if got := header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func assertNoCacheHeaders(t *testing.T, header http.Header) {
	t.Helper()
	expected := map[string]string{
		"Cache-Control": "no-store, no-cache, must-revalidate, max-age=0",
		"Pragma":        "no-cache",
		"Expires":       "0",
	}
	for name, want := range expected {
		if got := header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestHealth(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestHealthReportsDegradedTransport(t *testing.T) {
	provider := staticTransportStatus{status: transport.Status{
		Mode:       "serial",
		State:      transport.StateDegraded,
		Endpoint:   "/dev/ttyUSB0",
		LastError:  "device lost",
		RetryCount: 3,
	}}
	server := NewServer(
		testConfig(),
		"v0.0.0-test",
		events.NewBus(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		WithTransportStatus(provider),
	)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body struct {
		Status    string           `json:"status"`
		Transport transport.Status `json:"transport"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body.Status != "degraded" || body.Transport != provider.status {
		t.Fatalf("health = %+v, want degraded %+v", body, provider.status)
	}
}

func TestRequestLogEnabledLogsStructuredRequest(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(logfmt.New(&logBuffer, slog.LevelDebug)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	cfg := testConfig()
	cfg.RequestLog.Enabled = true
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.RemoteAddr = "198.51.100.12:4321"
	request.Header.Set("CF-Connecting-IP", "203.0.113.10")
	request.Header.Set("X-Real-IP", "198.51.100.99")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	logLine := logBuffer.String()
	for _, want := range []string{
		"http request",
		"method=GET",
		"endpoint=/api/health",
		"status=200",
		"caller_ip=203.0.113.10",
		"started_at=",
		"duration=",
		"duration_ms=",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("request log = %q, want %q", logLine, want)
		}
	}
}

func TestRequestLogDisabledDoesNotLogRequest(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(logfmt.New(&logBuffer, slog.LevelDebug)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if logBuffer.Len() != 0 {
		t.Fatalf("request log = %q, want empty", logBuffer.String())
	}
}

func TestRequestLogUsesRealIPWhenCloudflareHeaderMissing(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(logfmt.New(&logBuffer, slog.LevelDebug)))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	cfg := testConfig()
	cfg.RequestLog.Enabled = true
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set("X-Real-IP", "198.51.100.99")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if !strings.Contains(logBuffer.String(), "caller_ip=198.51.100.99") {
		t.Fatalf("request log = %q, want X-Real-IP", logBuffer.String())
	}
}

func TestCreateMessage(t *testing.T) {
	tests := map[string]struct {
		body       map[string]string
		bridgeErr  error
		wantStatus int
		wantCalls  int
	}{
		"valid": {
			body:       map[string]string{"dst": "*", "msg": "hello"},
			wantStatus: http.StatusAccepted,
			wantCalls:  1,
		},
		"invalid": {
			body:       map[string]string{"dst": "*", "msg": ""},
			wantStatus: http.StatusBadRequest,
			wantCalls:  0,
		},
		"bridge error": {
			body:       map[string]string{"dst": "QQ1ABC-1", "msg": "hi"},
			bridgeErr:  fmt.Errorf("udp timeout"),
			wantStatus: http.StatusBadGateway,
			wantCalls:  1,
		},
		"node not yet detected": {
			body:       map[string]string{"dst": "QQ1ABC-1", "msg": "hi"},
			bridgeErr:  udpbridge.ErrNodeNotDetected,
			wantStatus: http.StatusServiceUnavailable,
			wantCalls:  1,
		},
		"transport unavailable": {
			body:       map[string]string{"dst": "QQ1ABC-1", "msg": "hi"},
			bridgeErr:  fmt.Errorf("serial disconnected: %w", transport.ErrUnavailable),
			wantStatus: http.StatusServiceUnavailable,
			wantCalls:  1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(test.body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}

			bridge := &stubBridge{err: test.bridgeErr}
			server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, bridge, nil, nil)
			request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", response.Code, test.wantStatus, response.Body.String())
			}
			if int(bridge.calls.Load()) != test.wantCalls {
				t.Fatalf("bridge calls = %d, want %d", bridge.calls.Load(), test.wantCalls)
			}
		})
	}
}

func TestCreateMessagePersistsFailedWhenEchoMissing(t *testing.T) {
	cfg := testConfig()
	bus := events.NewBus()
	log := newHTTPChatLog(t, staticCallsign(cfg.MyCall))
	server := NewServer(cfg, "v0.0.0-test", bus, nil, nil, log, &stubBridge{}, nil, nil)
	server.outbox = outbox.New(10*time.Millisecond, server.handleOutgoingTimeout)

	body := []byte(`{"dst":"QQ1ABC-1","msg":"hello"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", response.Code, http.StatusAccepted, response.Body.String())
	}

	var records []chatlog.Record
	deadline := time.After(time.Second)
	for len(records) == 0 {
		var err error
		records, err = log.ReadSince("DM_QQ0QQ_QQ1ABC-1", time.Time{})
		if err != nil {
			t.Fatalf("ReadSince: %v", err)
		}
		select {
		case <-deadline:
			t.Fatal("failed record not persisted")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if records[0].DeliveryStatus != "failed" {
		t.Fatalf("DeliveryStatus = %q, want failed", records[0].DeliveryStatus)
	}
	if records[0].Direction != "outbound" {
		t.Fatalf("Direction = %q, want outbound", records[0].Direction)
	}
}

func TestCreateMessageDoesNotFailWhenEchoArrives(t *testing.T) {
	cfg := testConfig()
	bus := events.NewBus()
	log := newHTTPChatLog(t, staticCallsign(cfg.MyCall))
	server := NewServer(cfg, "v0.0.0-test", bus, nil, nil, log, &stubBridge{}, nil, nil)
	server.outbox = outbox.New(30*time.Millisecond, server.handleOutgoingTimeout)

	body := []byte(`{"dst":"QQ1ABC-1","msg":"hello"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", response.Code, http.StatusAccepted, response.Body.String())
	}

	bus.Publish(events.Event{
		Type: "packet.received",
		Data: map[string]any{
			"packet": meshcom.TextMessage{
				Source:      cfg.MyCall,
				Destination: "QQ1ABC-1",
				Message:     "hello{5712",
			},
		},
	})

	time.Sleep(60 * time.Millisecond)
	records, err := log.ReadSince("DM_QQ0QQ_QQ1ABC-1", time.Time{})
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want none", records)
	}
}

func TestCreateMessageRegistersOutboxBeforeSend(t *testing.T) {
	cfg := testConfig()
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	server.outbox = outbox.New(time.Minute, nil)
	confirmedDuringSend := false
	server.bridge = &stubBridge{onSend: func() {
		_, confirmedDuringSend = server.outbox.Confirm(cfg.MyCall, "QQ1ABC-1", "hello{571")
	}}

	body := []byte(`{"dst":"QQ1ABC-1","msg":"hello"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", response.Code, response.Body.String())
	}
	if !confirmedDuringSend {
		t.Fatal("outbox was not registered before transport send")
	}
}

func TestCreateMessageCancelsOutboxWhenSendFails(t *testing.T) {
	cfg := testConfig()
	failed := make(chan outbox.PendingMessage, 1)
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, &stubBridge{err: transport.ErrUnavailable}, nil, nil)
	server.outbox = outbox.New(20*time.Millisecond, func(message outbox.PendingMessage) {
		failed <- message
	})

	body := []byte(`{"dst":"QQ1ABC-1","msg":"hello"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", response.Code, response.Body.String())
	}
	select {
	case message := <-failed:
		t.Fatalf("failed transport left pending outbox message: %+v", message)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAuthProtectedRouteRequiresSession(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/chat/list", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(response.Body.String(), "unauthorized") {
		t.Fatalf("body = %q, want unauthorized error", response.Body.String())
	}
}

func TestAuthSessionLifecycle(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	loginBody, err := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(loginBody))
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", loginRec.Code, http.StatusNoContent)
	}

	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/chat/list", nil)
	protectedReq.AddCookie(cookies[0])
	protectedRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(protectedRec, protectedReq)

	if protectedRec.Code != http.StatusOK {
		t.Fatalf("protected status = %d, want %d", protectedRec.Code, http.StatusOK)
	}

	logoutReq := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	logoutReq.AddCookie(cookies[0])
	logoutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logoutRec.Code, http.StatusNoContent)
	}

	reuseReq := httptest.NewRequest(http.MethodGet, "/api/chat/list", nil)
	reuseReq.AddCookie(cookies[0])
	reuseRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reuseRec, reuseReq)

	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("reused session status = %d, want %d", reuseRec.Code, http.StatusUnauthorized)
	}
}

func TestAuthSessionSurvivesServerRestart(t *testing.T) {
	cfg := testConfig()
	cfg.DataDir = t.TempDir()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	sessionDB := openSessionTestDB(t)
	firstServer := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil, WithSessionDB(sessionDB))
	loginBody, err := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(loginBody))
	loginResponse := httptest.NewRecorder()
	firstServer.Handler().ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want %d", loginResponse.Code, http.StatusNoContent)
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	firstServer.Close()

	restartedServer := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil, WithSessionDB(sessionDB))
	defer restartedServer.Close()
	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	sessionRequest.AddCookie(cookies[0])
	sessionResponse := httptest.NewRecorder()
	restartedServer.Handler().ServeHTTP(sessionResponse, sessionRequest)

	if sessionResponse.Code != http.StatusOK {
		t.Fatalf("session status after restart = %d, want %d", sessionResponse.Code, http.StatusOK)
	}
}

func TestAuthRejectsInvalidCredentials(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	loginBody, err := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(loginBody))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestGetSessionStatus(t *testing.T) {
	t.Run("auth disabled", func(t *testing.T) {
		server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
		request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		response := httptest.NewRecorder()

		server.Handler().ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if !strings.Contains(response.Body.String(), `"authenticated":true`) {
			t.Fatalf("body = %q", response.Body.String())
		}
	})

	t.Run("auth enabled without cookie", func(t *testing.T) {
		cfg := testConfig()
		cfg.Auth = config.Auth{
			Username:   "admin",
			Password:   "secret",
			SessionTTL: time.Hour,
			CookieName: "meshcom_session",
		}

		server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
		request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		response := httptest.NewRecorder()

		server.Handler().ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
		if !strings.Contains(response.Body.String(), `"required":true`) {
			t.Fatalf("body = %q", response.Body.String())
		}
	})
}

func TestCreateMessageDedup(t *testing.T) {
	bridge := &stubBridge{}
	sc := sendcache.New(100 * time.Millisecond)
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, bridge, sc, nil)

	post := func(dst, msg string) int {
		body, _ := json.Marshal(map[string]string{"dst": dst, "msg": msg})
		req := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	// First send: accepted.
	if got := post("*", "hello"); got != http.StatusAccepted {
		t.Fatalf("first send status = %d, want 202", got)
	}
	if bridge.calls.Load() != 1 {
		t.Fatalf("bridge calls = %d, want 1", bridge.calls.Load())
	}

	// Duplicate within TTL: 429.
	if got := post("*", "hello"); got != http.StatusTooManyRequests {
		t.Fatalf("duplicate status = %d, want 429", got)
	}
	if bridge.calls.Load() != 1 {
		t.Fatalf("bridge calls after duplicate = %d, still want 1", bridge.calls.Load())
	}

	// Different dst: accepted.
	if got := post("QQ1ABC-1", "hello"); got != http.StatusAccepted {
		t.Fatalf("different dst status = %d, want 202", got)
	}

	// Different msg: accepted.
	if got := post("*", "world"); got != http.StatusAccepted {
		t.Fatalf("different msg status = %d, want 202", got)
	}

	// After TTL expiry: accepted again.
	time.Sleep(120 * time.Millisecond)
	if got := post("*", "hello"); got != http.StatusAccepted {
		t.Fatalf("post-TTL status = %d, want 202", got)
	}
}

func intPtr(value int) *int { return &value }

func TestListPositions(t *testing.T) {
	positionStore := newHTTPPositionsStore(t)
	seenAt := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	positionStore.Update(meshcom.Position{
		Source:    "QQ1ABC-1",
		Latitude:  48.1,
		Longitude: 16.3,
		Altitude:  123,
		RSSI:      intPtr(-90),
		SNR:       intPtr(8),
	}, seenAt)

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), positionStore, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body map[string]positions.Record
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode positions: %v", err)
	}
	if body["QQ1ABC-1"].Longitude != 16.3 {
		t.Fatalf("positions = %+v", body)
	}
	if body["QQ1ABC-1"].LastDirectSeen == nil || !body["QQ1ABC-1"].LastDirectSeen.Equal(seenAt) {
		t.Fatalf("positions lastdirectseen = %+v", body["QQ1ABC-1"].LastDirectSeen)
	}
}

func TestListConversations(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	log.Append(meshcom.TextMessage{
		Destination: "*",
		Message:     "hello",
	}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/chat/list", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body []chatlog.Conversation
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}
	if len(body) != 1 || body[0].ID != "P_broadcast" {
		t.Fatalf("conversations = %+v", body)
	}
}

func TestGetConversation(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	log.Append(meshcom.TextMessage{
		Destination: "*",
		Message:     "hello",
	}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	// Use the correct path format for Mux path values in tests
	// Actually ServeMux in Go 1.22+ needs the path to match the pattern
	request := httptest.NewRequest(http.MethodGet, "/api/chat/P_broadcast", nil)
	request.SetPathValue("conversation", "P_broadcast")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", response.Code, http.StatusOK, response.Body.String())
	}

	var body []chatlog.Record
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode records: %v", err)
	}
	if len(body) != 1 || body[0].Msg != "hello" {
		t.Fatalf("records = %+v", body)
	}
}

func TestGetConversationInvalid(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/chat/invalid!", nil)
	request.SetPathValue("conversation", "invalid!")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestGetConversationWithHours(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))

	now := time.Now().UTC()
	log.Append(meshcom.TextMessage{Destination: "*", Message: "recent"}, now.Add(-10*time.Minute))
	log.Append(meshcom.TextMessage{Destination: "*", Message: "old"}, now.Add(-2*time.Hour))

	cfg := testConfig()
	cfg.ChatLog.HistoryWindow = time.Hour
	cfg.ChatLog.MaxHistoryWindow = 24 * time.Hour

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	// Default window (1h) -> only "recent"
	req1 := httptest.NewRequest(http.MethodGet, "/api/chat/P_broadcast", nil)
	req1.SetPathValue("conversation", "P_broadcast")
	rec1 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec1, req1)

	var body1 []chatlog.Record
	json.Unmarshal(rec1.Body.Bytes(), &body1)
	if len(body1) != 1 {
		t.Errorf("default window count = %d, want 1", len(body1))
	}

	// Custom window (3h) -> both
	req2 := httptest.NewRequest(http.MethodGet, "/api/chat/P_broadcast?hours=3", nil)
	req2.SetPathValue("conversation", "P_broadcast")
	rec2 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec2, req2)

	var body2 []chatlog.Record
	json.Unmarshal(rec2.Body.Bytes(), &body2)
	if len(body2) != 2 {
		t.Errorf("custom window count = %d, want 2", len(body2))
	}
}

func TestGetConversationUsesThirtyDaysForDMDefault(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))

	now := time.Now().UTC()
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "dm recent"}, now.Add(-29*24*time.Hour))
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "dm old"}, now.Add(-31*24*time.Hour))
	log.Append(meshcom.TextMessage{Destination: "*", Message: "broadcast old"}, now.Add(-2*time.Hour))

	cfg := testConfig()
	cfg.ChatLog.HistoryWindow = time.Hour
	cfg.ChatLog.MaxHistoryWindow = 30 * 24 * time.Hour

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	// mycall-scoped id: DM_<basecall(mycall)>-1_<peer> = DM_QQ0QQ-1_QQ1ABC-1
	// FileIDForAPIID resolves to DM_QQ0QQ_QQ1ABC-1 (the actual file).
	dmRequest := httptest.NewRequest(http.MethodGet, "/api/chat/DM_QQ0QQ-1_QQ1ABC-1", nil)
	dmRequest.SetPathValue("conversation", "DM_QQ0QQ-1_QQ1ABC-1")
	dmResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(dmResponse, dmRequest)

	var dmBody []chatlog.Record
	if err := json.Unmarshal(dmResponse.Body.Bytes(), &dmBody); err != nil {
		t.Fatalf("decode dm records: %v", err)
	}
	if len(dmBody) != 1 || dmBody[0].Msg != "dm recent" {
		t.Fatalf("dm records = %+v", dmBody)
	}

	broadcastRequest := httptest.NewRequest(http.MethodGet, "/api/chat/P_broadcast", nil)
	broadcastRequest.SetPathValue("conversation", "P_broadcast")
	broadcastResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(broadcastResponse, broadcastRequest)

	var broadcastBody []chatlog.Record
	if err := json.Unmarshal(broadcastResponse.Body.Bytes(), &broadcastBody); err != nil {
		t.Fatalf("decode broadcast records: %v", err)
	}
	if len(broadcastBody) != 0 {
		t.Fatalf("broadcast records = %+v, want empty", broadcastBody)
	}
}

func TestSPARouting(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	// Requesting root should serve SPA
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec2, req2)
	// Status should be 200 or 404 depending on if FS is empty, but not 500
	if rec2.Code == http.StatusInternalServerError {
		t.Errorf("root status = %d", rec2.Code)
	}

	// Unknown non-API path should also return 200 (fallback to index.html)
	req3 := httptest.NewRequest(http.MethodGet, "/some/ui/route", nil)
	rec3 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec3, req3)
	if rec3.Code == http.StatusInternalServerError {
		t.Errorf("spa fallback status = %d", rec3.Code)
	}
}

func TestStreamEventsHeartbeatAndIdentity(t *testing.T) {
	positionStore := newHTTPPositionsStore(t)
	positionStore.Update(meshcom.Position{
		Source:    "QQ1ABC-1",
		Latitude:  48.1,
		Longitude: 16.3,
		RSSI:      intPtr(-90),
		SNR:       intPtr(8),
	}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), positionStore, nil, nil, nil, nil, nil)
	body := streamBodyUntil(t, server, "event: positions.snapshot")
	if !strings.Contains(body, "event: positions.snapshot") {
		t.Fatalf("snapshot event missing from stream body: %q", body)
	}
}

func TestStreamEventsStartsWithHeartbeat(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	body := streamBodyUntil(t, server, "event: heartbeat")
	if !strings.HasPrefix(body, "event: heartbeat\ndata: {}\n\n") {
		t.Fatalf("stream body prefix = %q, want heartbeat event", body)
	}
}

func TestStreamEventsRequiresSessionWhenAuthEnabled(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestStreamEventsSendsStationIdentity(t *testing.T) {
	cfg := testConfig()
	cfg.MyCall = "QQ1ABC-7"
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	body := streamBodyUntil(t, server, "event: station.identity")
	if !strings.Contains(body, `"callsign":"QQ1ABC-7"`) {
		t.Fatalf("station identity missing callsign: %q", body)
	}
}

func TestStreamEventsSendsStationIdentityTxDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.MyCall = "QQ1ABC-7"
	cfg.DemoMode = true
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	body := streamBodyUntil(t, server, "event: station.identity")
	if !strings.Contains(body, `"txDisabled":true`) {
		t.Fatalf("station identity missing txDisabled:true: %q", body)
	}
}

func TestStreamEventsStationIdentityTxEnabledByDefault(t *testing.T) {
	cfg := testConfig()
	cfg.MyCall = "QQ1ABC-7"
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	body := streamBodyUntil(t, server, "event: station.identity")
	if !strings.Contains(body, `"txDisabled":false`) {
		t.Fatalf("station identity missing txDisabled:false: %q", body)
	}
	if !strings.Contains(body, `"forwardTargetCount":0`) {
		t.Fatalf("station identity missing forwardTargetCount:0: %q", body)
	}
}

func TestStreamEventsReplaysRecentReceiveLogPackets(t *testing.T) {
	logger := newHTTPReceiveLog(t, receivelog.Config{Enabled: true})
	recentTime := time.Now().UTC().Add(-2 * time.Minute)
	if err := logger.Append(receivelog.Record{
		ReceivedAt: recentTime,
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"type":"msg","src":"QQ1ABC-1","dst":"*","msg":"hello"}`,
		PacketType: "msg",
	}); err != nil {
		t.Fatalf("append recent record: %v", err)
	}
	if err := logger.Append(receivelog.Record{
		ReceivedAt: time.Now().UTC().Add(-time.Minute),
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"src_type":"node","type":"pos","src":"POS1","msg":"","lat":48,"lat_dir":"N","long":16,"long_dir":"E","aprs_symbol":"#","aprs_symbol_group":"/","hw_id":"MAC","msg_id":"ABC","alt":123,"batt":85,"firmware":"4.35","fw_sub":"p","rssi":-90,"snr":8}`,
		PacketType: "pos",
	}); err != nil {
		t.Fatalf("append recent position record: %v", err)
	}
	if err := logger.Append(receivelog.Record{
		ReceivedAt: time.Now().UTC().Add(-7 * time.Hour),
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"type":"msg","src":"OLD","dst":"*","msg":"old"}`,
		PacketType: "msg",
	}); err != nil {
		t.Fatalf("append old record: %v", err)
	}

	cfg := testConfig()
	cfg.ReceiveLog = config.ReceiveLog{
		Enabled:      true,
		ReplayWindow: 6 * time.Hour,
	}
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, logger, nil, nil, nil, nil)
	body := streamBodyUntil(t, server, "QQ1ABC-1")

	if !strings.Contains(body, "event: packet.received") {
		t.Fatalf("replay event missing from stream body: %q", body)
	}
	if !strings.Contains(body, "QQ1ABC-1") {
		t.Fatalf("recent packet missing from stream body: %q", body)
	}
	if !strings.Contains(body, `"replay":true`) {
		t.Fatalf("replay marker missing from stream body: %q", body)
	}
	if !strings.Contains(body, `"received_at":"`+recentTime.Format(time.RFC3339Nano)+`"`) {
		t.Fatalf("replay received_at missing from stream body: %q", body)
	}
	if strings.Contains(body, "OLD") {
		t.Fatalf("old packet replayed: %q", body)
	}
}

func TestStreamEventsReplayFromQueryCappedByReplayWindow(t *testing.T) {
	logger := newHTTPReceiveLog(t, receivelog.Config{Enabled: true})
	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	if err := logger.Append(receivelog.Record{
		ReceivedAt: oldTime,
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"type":"msg","src":"OLDIN","dst":"*","msg":"old"}`,
		PacketType: "msg",
	}); err != nil {
		t.Fatalf("append old record: %v", err)
	}

	cfg := testConfig()
	cfg.ReceiveLog = config.ReceiveLog{
		Enabled:      true,
		ReplayWindow: time.Hour,
	}
	bus := events.NewBus()
	server := NewServer(cfg, "v0.0.0-test", bus, nil, logger, nil, nil, nil, nil)
	from := oldTime.Add(-time.Minute).Format(time.RFC3339Nano)

	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish(events.Event{Type: "packet.received", Data: "LIVEMARKER"})
	}()

	// Requesting 'from' 2h1m ago, but ReplayWindow is 1h. It should cap to 1h, so OLDIN (2h ago) is not replayed.
	body := streamBodyUntilPath(t, server, "/api/events?from="+from, "LIVEMARKER")

	if strings.Contains(body, "OLDIN") {
		t.Fatalf("packet older than ReplayWindow was replayed: %q", body)
	}
}

func TestStreamEventsReplayFromQueryWithinReplayWindow(t *testing.T) {
	logger := newHTTPReceiveLog(t, receivelog.Config{Enabled: true})

	// Packet 1: 90 minutes ago (within 2h ReplayWindow)
	packet1Time := time.Now().UTC().Add(-90 * time.Minute)
	if err := logger.Append(receivelog.Record{
		ReceivedAt: packet1Time,
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"type":"msg","src":"PACKET1","dst":"*","msg":"p1"}`,
		PacketType: "msg",
	}); err != nil {
		t.Fatalf("append record 1: %v", err)
	}

	// Packet 2: 30 minutes ago (within 2h ReplayWindow)
	packet2Time := time.Now().UTC().Add(-30 * time.Minute)
	if err := logger.Append(receivelog.Record{
		ReceivedAt: packet2Time,
		RemoteAddr: "127.0.0.1:1799",
		Raw:        `{"type":"msg","src":"PACKET2","dst":"*","msg":"p2"}`,
		PacketType: "msg",
	}); err != nil {
		t.Fatalf("append record 2: %v", err)
	}

	cfg := testConfig()
	cfg.ReceiveLog = config.ReceiveLog{
		Enabled:      true,
		ReplayWindow: 2 * time.Hour,
	}
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, logger, nil, nil, nil, nil)

	// Query 'from' 1 hour ago. Only Packet 2 (30m ago) should be replayed. Packet 1 (90m ago) should be filtered out.
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	body := streamBodyUntilPath(t, server, "/api/events?from="+from, "PACKET2")

	if strings.Contains(body, "PACKET1") {
		t.Fatalf("packet older than 'from' query parameter was replayed: %q", body)
	}
	if !strings.Contains(body, "PACKET2") {
		t.Fatalf("packet newer than 'from' query parameter was not replayed: %q", body)
	}
}

func TestStreamEventsRejectsInvalidReplayFromQuery(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/events?from=bad", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if !strings.Contains(response.Body.String(), "invalid from timestamp") {
		t.Fatalf("body = %q, want invalid from error", response.Body.String())
	}
}

func TestDeleteConversation(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	log.Append(meshcom.TextMessage{Destination: "*", Message: "hello"}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/api/chat/P_broadcast", nil)
	request.SetPathValue("conversation", "P_broadcast")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", response.Code, response.Body.String())
	}

	convs, _ := log.List()
	for _, c := range convs {
		if c.ID == "P_broadcast" {
			t.Fatal("P_broadcast still in List after delete")
		}
	}
}

func TestDeleteConversationInvalidID(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/api/chat/invalid!", nil)
	request.SetPathValue("conversation", "invalid!")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestDeleteConversationMissingFile(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/api/chat/P_999", nil)
	request.SetPathValue("conversation", "P_999")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (idempotent)", response.Code)
	}
}

func TestDeleteBroadcast(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	log.Append(meshcom.TextMessage{Destination: "*", Message: "test"}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)
	request := httptest.NewRequest(http.MethodDelete, "/api/chat/P_broadcast", nil)
	request.SetPathValue("conversation", "P_broadcast")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}

	records, _ := log.ReadSince("P_broadcast", time.Time{})
	if len(records) != 0 {
		t.Fatalf("records after broadcast delete = %d, want 0", len(records))
	}
}

func TestServerClose(t *testing.T) {
	bus := events.NewBus()
	server := NewServer(testConfig(), "v0.0.0-test", bus, nil, nil, nil, nil, nil, nil)

	// Register a message in outbox
	server.outbox.Register("SRC", "DST", "HELLO", time.Now())

	// Close the server to cancel the background watch goroutine
	server.Close()
	time.Sleep(50 * time.Millisecond) // Let the cancellation goroutine run

	// Publish the packet.received event which would normally confirm and remove the message from outbox
	bus.Publish(events.Event{
		Type: "packet.received",
		Data: map[string]any{
			"packet": meshcom.TextMessage{
				Source:      "SRC",
				Destination: "DST",
				Message:     "HELLO",
			},
		},
	})
	time.Sleep(50 * time.Millisecond)

	// Confirm that the message in outbox was NOT confirmed (still exists), because the watch goroutine was stopped.
	if _, ok := server.outbox.Confirm("SRC", "DST", "HELLO"); !ok {
		t.Error("expected message to still be pending in outbox after Close()")
	}
}

type safeResponseWriter struct {
	mu  sync.Mutex
	rec *httptest.ResponseRecorder
}

func (s *safeResponseWriter) Header() http.Header {
	return s.rec.Header()
}

func (s *safeResponseWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Write(b)
}

func (s *safeResponseWriter) WriteHeader(statusCode int) {
	s.rec.WriteHeader(statusCode)
}

func (s *safeResponseWriter) Flush() {
	s.rec.Flush()
}

func (s *safeResponseWriter) BodyString() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Body.String()
}

func streamBodyUntil(t *testing.T, server *Server, marker string) string {
	t.Helper()
	return streamBodyUntilPath(t, server, "/api/events", marker)
}

func streamBodyUntilPath(t *testing.T, server *Server, path string, marker string) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	safeWriter := &safeResponseWriter{rec: response}

	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(safeWriter, request)
		close(done)
	}()

	deadline := time.After(time.Second)
	for {
		body := safeWriter.BodyString()
		if strings.Contains(body, marker) {
			cancel()
			<-done
			return body
		}

		select {
		case <-deadline:
			t.Fatalf("stream marker %q missing from body: %q", marker, body)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		MyCall:           "QQ0QQ-1",
		MaxMessageLength: 149,
		ChatLog: config.ChatLog{
			HistoryWindow:    24 * time.Hour,
			MaxHistoryWindow: 7 * 24 * time.Hour,
		},
	}
}

func testTime() time.Time {
	return time.Now().UTC().Add(-time.Hour)
}

func newTestChatStatus(t *testing.T) *chatstatus.Store {
	t.Helper()
	store, _ := newTestChatStatusWithDB(t)
	return store
}

func newTestChatStatusWithDB(t *testing.T) (*chatstatus.Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "gomeshcom.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE chat_reads (conversation_id TEXT PRIMARY KEY, last_read TEXT NOT NULL)`,
		`CREATE TABLE chats_public (id INTEGER PRIMARY KEY AUTOINCREMENT, conversation_id TEXT NOT NULL, kind TEXT NOT NULL, channel TEXT, msg_id TEXT, received_at TEXT NOT NULL, src TEXT, src_type TEXT, via TEXT, dst TEXT NOT NULL, msg TEXT NOT NULL, rssi INTEGER, snr INTEGER)`,
		`CREATE TABLE chats_dm (id INTEGER PRIMARY KEY AUTOINCREMENT, conversation_id TEXT NOT NULL, msg_id TEXT, sequence_id TEXT, received_at TEXT NOT NULL, src TEXT, src_type TEXT, via TEXT, dst TEXT NOT NULL, msg TEXT NOT NULL, rssi INTEGER, snr INTEGER, direction TEXT, delivery_status TEXT, ack_status TEXT, ack_received_at TEXT, ack_src TEXT, ack_src_type TEXT, ack_rssi INTEGER, ack_snr INTEGER, ack_via TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("create chatstatus table: %v", err)
		}
	}
	s, err := chatstatus.NewSQLite(db)
	if err != nil {
		t.Fatalf("chatstatus.NewSQLite: %v", err)
	}
	return s, db
}

func insertChatStatusPublicRow(t *testing.T, db *sql.DB, convID string, receivedAt time.Time, msg string) {
	t.Helper()
	kind := "broadcast"
	var channel any
	if strings.HasPrefix(convID, "P_") && convID != "P_broadcast" {
		kind = "channel"
		channel = strings.TrimPrefix(convID, "P_")
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO chats_public(conversation_id, kind, channel, received_at, dst, msg) VALUES (?, ?, ?, ?, '*', ?)`, convID, kind, channel, receivedAt.UTC().Format(time.RFC3339Nano), msg); err != nil {
		t.Fatalf("insert public chat row: %v", err)
	}
}

func insertChatStatusDMRow(t *testing.T, db *sql.DB, convID string, src string, dst string, receivedAt time.Time, msg string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO chats_dm(conversation_id, received_at, src, dst, msg) VALUES (?, ?, ?, ?, ?)`, convID, receivedAt.UTC().Format(time.RFC3339Nano), src, dst, msg); err != nil {
		t.Fatalf("insert dm chat row: %v", err)
	}
}

func TestDeleteConversationRemovesChatStatusEntry(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	cs := newTestChatStatus(t)
	cs.MarkRead("P_broadcast", time.Now())
	if err := cs.SaveIfDirty(); err != nil {
		t.Fatalf("SaveIfDirty: %v", err)
	}

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, cs)

	req := httptest.NewRequest(http.MethodDelete, "/api/chat/P_broadcast", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	snap := cs.Snapshot()
	if _, present := snap["P_broadcast"]; present {
		t.Fatalf("chatstatus entry for P_broadcast must be removed after DELETE, still has UnreadCount=%d", snap["P_broadcast"].UnreadCount)
	}
}

func TestMarkConversationRead(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	cs, statusDB := newTestChatStatusWithDB(t)
	insertChatStatusPublicRow(t, statusDB, "P_broadcast", time.Now(), "test")

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, cs)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/P_broadcast/read", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	snap := cs.Snapshot()
	if snap["P_broadcast"].UnreadCount != 0 {
		t.Fatalf("UnreadCount = %d, want 0 after MarkRead", snap["P_broadcast"].UnreadCount)
	}
	if snap["P_broadcast"].LastRead.IsZero() {
		t.Fatal("LastRead must be set after MarkRead")
	}
}

func TestMarkConversationReadInvalidID(t *testing.T) {
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/INVALID_ID!/read", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMarkConversationReadRequiresAuth(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/P_broadcast/read", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestGetConversationDoesNotAlterUnreadCount(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	cs, statusDB := newTestChatStatusWithDB(t)
	insertChatStatusPublicRow(t, statusDB, "P_broadcast", time.Now(), "test")

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, cs)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/P_broadcast", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	snap := cs.Snapshot()
	if snap["P_broadcast"].UnreadCount != 1 {
		t.Fatalf("GET must not alter UnreadCount: got %d, want 1", snap["P_broadcast"].UnreadCount)
	}
}

func TestStreamEventsChatStatusSnapshot(t *testing.T) {
	cs, statusDB := newTestChatStatusWithDB(t)
	now := time.Now()
	insertChatStatusPublicRow(t, statusDB, "P_broadcast", now, "test")
	insertChatStatusPublicRow(t, statusDB, "P_broadcast", now.Add(time.Second), "test")

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, cs)
	body := streamBodyUntil(t, server, "event: chatstatus.snapshot")

	if !strings.Contains(body, "event: chatstatus.snapshot") {
		t.Fatalf("chatstatus.snapshot missing from SSE stream: %q", body)
	}
	if !strings.Contains(body, `"unreadCount":2`) {
		t.Fatalf("chatstatus snapshot missing unreadCount:2: %q", body)
	}
}

func TestCreateMessageRejectsTooLargeBody(t *testing.T) {
	bridge := &stubBridge{}
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, nil, bridge, nil, nil)

	body := bytes.Repeat([]byte("x"), 1<<14) // 16 KB > 8 KB limit
	request := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized body", response.Code)
	}
	if bridge.calls.Load() != 0 {
		t.Fatal("bridge called despite oversized body")
	}
}

func TestCreateSessionRejectsTooLargeBody(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = config.Auth{
		Username:   "admin",
		Password:   "secret",
		SessionTTL: time.Hour,
		CookieName: "meshcom_session",
	}

	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)

	body := bytes.Repeat([]byte("x"), 1<<11) // 2 KB > 1 KB limit
	request := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized body", response.Code)
	}
}

func TestRequestLogUsesXForwardedForWhenCFHeaderMissing(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(logfmt.New(&logBuffer, slog.LevelDebug)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	cfg := testConfig()
	cfg.RequestLog.Enabled = true
	server := NewServer(cfg, "v0.0.0-test", events.NewBus(), nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set("X-Forwarded-For", "198.51.100.42")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if !strings.Contains(logBuffer.String(), "caller_ip=198.51.100.42") {
		t.Fatalf("request log = %q, want X-Forwarded-For IP", logBuffer.String())
	}
}

// ---- scope-aware API tests --------------------------------------------------

func TestListConversationsMycallScope(t *testing.T) {
	// testConfig().MyCall = "QQ0QQ-1", basecall = "QQ0QQ"
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	// Write a DM from peer to us: file lands in DM_QQ0QQ_QQ1ABC-1.jsonl
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "hi"}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/list?scope=mycall", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []chatlog.Conversation
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(body))
	}
	// mycall scope: id must use full SSID form DM_QQ0QQ-1_QQ1ABC-1
	if body[0].ID != "DM_QQ0QQ-1_QQ1ABC-1" {
		t.Errorf("ID = %q, want DM_QQ0QQ-1_QQ1ABC-1", body[0].ID)
	}
}

func TestListConversationsBasecallScope(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "hi"}, testTime())

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/list?scope=basecall", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []chatlog.Conversation
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(body))
	}
	// basecall scope: id keeps file id form DM_QQ0QQ_QQ1ABC-1
	if body[0].ID != "DM_QQ0QQ_QQ1ABC-1" {
		t.Errorf("ID = %q, want DM_QQ0QQ_QQ1ABC-1", body[0].ID)
	}
}

func TestListConversationsMycallScopeExcludesOtherSSID(t *testing.T) {
	// Write a DM from peer to QQ0QQ-2, landing in the shared DM_QQ0QQ_QQ1ABC-1.jsonl.
	logOther := newHTTPChatLog(t, staticCallsign("QQ0QQ-2"))
	logOther.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-2", Message: "hi"}, testTime())

	// Server is configured as QQ0QQ-1 — a different SSID sharing the same basecall.
	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, logOther, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/list?scope=mycall", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body []chatlog.Conversation
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, conv := range body {
		if strings.HasPrefix(conv.ID, "DM_") {
			t.Errorf("mycall scope returned DM conversation %q — file has no records for QQ0QQ-1", conv.ID)
		}
	}
}

func TestGetConversationMycallScopeFiltersRecords(t *testing.T) {
	// testConfig().MyCall = "QQ0QQ-1"
	db := openHTTPChatLogDB(t)
	log := newHTTPChatLogWithDB(t, db, staticCallsign("QQ0QQ-1"))
	now := time.Now().UTC()
	// Both messages land in DM_QQ0QQ_QQ1ABC-1.jsonl (basecall file).
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "to ssid-1"}, now.Add(-time.Minute))
	// Simulate a record from peer to a different device of ours (QQ0QQ-2).
	// We write directly via the chatlog to bypass filter.
	logOther := newHTTPChatLogWithDB(t, db, staticCallsign("QQ0QQ-2"))
	logOther.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-2", Message: "to ssid-2"}, now)

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	// mycall scope with our SSID: only QQ0QQ-1 records
	req := httptest.NewRequest(http.MethodGet, "/api/chat/DM_QQ0QQ-1_QQ1ABC-1?scope=mycall", nil)
	req.SetPathValue("conversation", "DM_QQ0QQ-1_QQ1ABC-1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var records []chatlog.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, r := range records {
		if r.Dst == "QQ0QQ-2" {
			t.Errorf("mycall scope returned record addressed to QQ0QQ-2: %+v", r)
		}
	}
	if len(records) == 0 {
		t.Error("expected at least one record for QQ0QQ-1")
	}
}

func TestGetConversationBasecallScopeReturnsAll(t *testing.T) {
	db := openHTTPChatLogDB(t)
	log := newHTTPChatLogWithDB(t, db, staticCallsign("QQ0QQ-1"))
	now := time.Now().UTC()
	log.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-1", Message: "hi1"}, now.Add(-2*time.Minute))

	// Write a second record directly to the same basecall file from a different SSID logger.
	logOther := newHTTPChatLogWithDB(t, db, staticCallsign("QQ0QQ-2"))
	logOther.Append(meshcom.TextMessage{Source: "QQ1ABC-1", Destination: "QQ0QQ-2", Message: "hi2"}, now.Add(-time.Minute))

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, nil)

	// basecall scope: returns all records from the shared file
	req := httptest.NewRequest(http.MethodGet, "/api/chat/DM_QQ0QQ_QQ1ABC-1?scope=basecall", nil)
	req.SetPathValue("conversation", "DM_QQ0QQ_QQ1ABC-1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var records []chatlog.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("basecall scope: expected 2 records, got %d", len(records))
	}
}

func TestMarkConversationReadBasecallScope(t *testing.T) {
	log := newHTTPChatLog(t, staticCallsign("QQ0QQ-1"))
	cs, statusDB := newTestChatStatusWithDB(t)

	// Two per-SSID status keys for the same peer.
	baseTime := time.Now().Add(-time.Minute)
	insertChatStatusDMRow(t, statusDB, "DM_QQ0QQ_QQ1ABC-1", "QQ1ABC-1", "QQ0QQ-1", baseTime, "msg1")
	insertChatStatusDMRow(t, statusDB, "DM_QQ0QQ_QQ1ABC-1", "QQ1ABC-1", "QQ0QQ-2", baseTime.Add(time.Second), "msg2")

	server := NewServer(testConfig(), "v0.0.0-test", events.NewBus(), nil, nil, log, nil, nil, cs)

	// basecall scope markRead on basecall-form id
	req := httptest.NewRequest(http.MethodPost, "/api/chat/DM_QQ0QQ_QQ1ABC-1/read?scope=basecall", nil)
	req.SetPathValue("conversation", "DM_QQ0QQ_QQ1ABC-1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	snap := cs.Snapshot()
	if e, ok := snap["DM_QQ0QQ-1_QQ1ABC-1"]; !ok || e.UnreadCount != 0 {
		t.Errorf("DM_QQ0QQ-1_QQ1ABC-1 UnreadCount = %v, want 0", snap["DM_QQ0QQ-1_QQ1ABC-1"])
	}
	if e, ok := snap["DM_QQ0QQ-2_QQ1ABC-1"]; !ok || e.UnreadCount != 0 {
		t.Errorf("DM_QQ0QQ-2_QQ1ABC-1 UnreadCount = %v, want 0", snap["DM_QQ0QQ-2_QQ1ABC-1"])
	}
}

// TestListDMStats verifies the /api/stats/dm endpoint: nil store returns empty
// map; after a send + ack echo the full/base entries are populated correctly.
func TestListDMStats(t *testing.T) {
	t.Run("nil store returns empty object", func(t *testing.T) {
		server := NewServer(testConfig(), "v0.0.0-test", nil, nil, nil, nil, nil, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/api/stats/dm", nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body map[string]dmStatsEntry
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("expected empty map, got %v", body)
		}
	})

	t.Run("records sent and ack via echo", func(t *testing.T) {
		cfg := testConfig()
		bus := events.NewBus()
		dmStore := dmstats.NewSQLite(openHTTPDMStatsDB(t))

		server := NewServer(cfg, "v0.0.0-test", bus, nil, nil, nil, &stubBridge{}, nil, nil,
			WithDMStats(dmStore),
		)
		// Override the outbox with a longer TTL so the pending message is not
		// expired before the echo arrives.
		server.outbox = outbox.New(5*time.Second, server.handleOutgoingTimeout)

		// Send a DM to QQ1ABC-1 (has SSID).
		sendBody := []byte(`{"dst":"QQ1ABC-1","msg":"hello"}`)
		sendReq := httptest.NewRequest(http.MethodPost, "/api/messages", bytes.NewReader(sendBody))
		sendRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(sendRec, sendReq)
		if sendRec.Code != http.StatusAccepted {
			t.Fatalf("send status = %d, want 202", sendRec.Code)
		}

		// Wait at least 1ms so the latency calculation produces a non-zero value.
		time.Sleep(2 * time.Millisecond)

		// Simulate the echo packet that triggers ack.
		bus.Publish(events.Event{
			Type: "packet.received",
			Data: map[string]any{
				"packet": meshcom.TextMessage{
					Source:      cfg.MyCall,
					Destination: "QQ1ABC-1",
					Message:     "hello",
				},
			},
		})

		// Wait for watchOutgoingEchoes goroutine to process the event.
		deadline := time.After(time.Second)
		for {
			snap := dmStore.Snapshot()
			if snap["QQ1ABC-1"].Ack > 0 {
				break
			}
			select {
			case <-deadline:
				t.Fatal("ack not recorded in dmstats within 1s")
			case <-time.After(5 * time.Millisecond):
			}
		}

		// Query the endpoint.
		req := httptest.NewRequest(http.MethodGet, "/api/stats/dm", nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var body map[string]dmStatsEntry
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Full entry: QQ1ABC-1 — sent=1, ack=1.
		full, ok := body["QQ1ABC-1"]
		if !ok {
			t.Fatalf("missing QQ1ABC-1 in response: %v", body)
		}
		if full.Sent != 1 {
			t.Errorf("QQ1ABC-1 sent: want 1, got %d", full.Sent)
		}
		if full.Ack != 1 {
			t.Errorf("QQ1ABC-1 ack: want 1, got %d", full.Ack)
		}

		// Base entry: QQ1ABC — sent=1, ack=1.
		base, ok := body["QQ1ABC"]
		if !ok {
			t.Fatalf("missing QQ1ABC base entry in response: %v", body)
		}
		if base.Sent != 1 {
			t.Errorf("QQ1ABC sent: want 1, got %d", base.Sent)
		}
		if base.Ack != 1 {
			t.Errorf("QQ1ABC ack: want 1, got %d", base.Ack)
		}
	})
}
