package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/config"
	"github.com/logocomune/gomeshcom-client/internal/events"
)

// fullTestConfig returns a valid config suitable for configapi tests.
func fullTestConfig() config.Config {
	return config.Config{
		HTTPAddr:         "127.0.0.1:8080",
		TransportMode:    config.TransportUDP,
		UDPListenAddr:    "0.0.0.0:1799",
		MyCall:           "QQ0QQ-1",
		DataDir:          "./data",
		MaxMessageLength: 149,
		LogLevel:         "info",
		ChatLog: config.ChatLog{
			Path:             "./data/chat",
			HistoryWindow:    24 * time.Hour,
			MaxHistoryWindow: 720 * time.Hour,
		},
		ReceiveLog: config.ReceiveLog{
			Enabled:       true,
			Path:          "./data/raw",
			RetentionDays: 365,
			ReplayWindow:  time.Hour,
		},
		Stats: config.Stats{
			Enabled:       true,
			Path:          "./data/stats/stats.json",
			RetentionDays: 30,
		},
		Send:       config.Send{DedupTTL: 2 * time.Second},
		Auth:       config.Auth{SessionTTL: 24 * time.Hour, CookieName: "meshcom_session"},
		RequestLog: config.RequestLog{Enabled: false},
		Serial: config.Serial{
			Baud:             115200,
			DataBits:         8,
			Parity:           "none",
			StopBits:         1,
			FlowControl:      "none",
			ReadTimeout:      time.Second,
			ReconnectInitial: time.Second,
			ReconnectMax:     30 * time.Second,
			StableResetAfter: 30 * time.Second,
			MaxRecordBytes:   65536,
		},
		NetConsole: config.NetConsole{
			ConnectTimeout:   5 * time.Second,
			AuthTimeout:      5 * time.Second,
			WriteTimeout:     5 * time.Second,
			ReconnectInitial: time.Second,
			ReconnectMax:     30 * time.Second,
			StableResetAfter: 30 * time.Second,
			MaxAuthLineBytes: 128,
			MaxRecordBytes:   65536,
		},
		Storage: config.Storage{
			SQLitePath:          "./data/gomeshcom.db",
			PurgeInterval:       4 * time.Hour,
			ReceiveLogRetention: 30 * 24 * time.Hour,
			PublicChatRetention: 30 * 24 * time.Hour,
			NodesRetention:      7 * 24 * time.Hour,
			TelemetryRetention:  30 * 24 * time.Hour,
		},
	}
}

func configTestServer(cfg config.Config, env config.EnvOverrides, tomlDir string) *Server {
	tomlPath := ""
	if tomlDir != "" {
		tomlPath = filepath.Join(tomlDir, "gomeshcomd.toml")
	}
	return NewServer(cfg, "v-test", events.NewBus(), nil, nil, nil, nil, nil, nil,
		WithEnvOverrides(env),
		WithTomlPath(tomlPath),
	)
}

// -------- buildConfigResponse (unit tests, no HTTP) --------

func TestBuildConfigResponsePasswordMasked(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Auth.Password = "supersecret"
	resp := buildConfigResponse(cfg, config.EnvOverrides{}, "", time.Time{})
	if resp.Auth.Password.Value == "supersecret" {
		t.Error("auth.password must not be the real value")
	}
	if resp.Auth.Password.Value != passwordMask {
		t.Errorf("auth.password.value = %v, want %q", resp.Auth.Password.Value, passwordMask)
	}
}

func TestBuildConfigResponseEmptyPasswordNotMasked(t *testing.T) {
	cfg := fullTestConfig()
	resp := buildConfigResponse(cfg, config.EnvOverrides{}, "", time.Time{})
	if resp.Auth.Password.Value != "" {
		t.Errorf("auth.password.value = %v, want empty string", resp.Auth.Password.Value)
	}
}

func TestBuildConfigResponseEnvOverrideMarked(t *testing.T) {
	cfg := fullTestConfig()
	env := config.EnvOverrides{"MY_CALL": true}
	resp := buildConfigResponse(cfg, env, "", time.Time{})
	if !resp.MyCall.EnvOverride {
		t.Error("my_call.env_override should be true when env var is set")
	}
	if resp.LogLevel.EnvOverride {
		t.Error("log_level.env_override should be false when env var not set")
	}
}

func TestBuildConfigResponseRequiresRestartFlag(t *testing.T) {
	cfg := fullTestConfig()
	resp := buildConfigResponse(cfg, config.EnvOverrides{}, "", time.Time{})
	if !resp.HTTPAddr.RequiresRestart {
		t.Error("http_addr.requires_restart should be true")
	}
	if resp.MyCall.RequiresRestart {
		t.Error("my_call.requires_restart should be false (live-apply)")
	}
}

func TestBuildConfigResponseIncludesStoragePurgeFields(t *testing.T) {
	cfg := fullTestConfig()
	env := config.EnvOverrides{"STORAGE_SQLITE_PATH": true, "STORAGE_NODES_RETENTION": true, "STORAGE_TELEMETRY_RETENTION": true}
	resp := buildConfigResponse(cfg, env, "", time.Time{})

	if resp.Storage.SQLitePath.Value != "./data/gomeshcom.db" {
		t.Fatalf("storage.sqlite_path.value = %v, want ./data/gomeshcom.db", resp.Storage.SQLitePath.Value)
	}
	if !resp.Storage.SQLitePath.EnvOverride {
		t.Fatal("storage.sqlite_path.env_override = false, want true")
	}
	if !resp.Storage.SQLitePath.RequiresRestart {
		t.Fatal("storage.sqlite_path.requires_restart = false, want true")
	}
	if resp.Storage.PurgeInterval.Value != "4h0m0s" {
		t.Fatalf("storage.purge_interval.value = %v, want 4h0m0s", resp.Storage.PurgeInterval.Value)
	}
	if resp.Storage.ReceiveLogRetention.Value != "720h0m0s" {
		t.Fatalf("storage.receive_log_retention.value = %v, want 720h0m0s", resp.Storage.ReceiveLogRetention.Value)
	}
	if !resp.Storage.NodesRetention.EnvOverride {
		t.Fatal("storage.nodes_retention.env_override = false, want true")
	}
	if resp.Storage.TelemetryRetention.Value != "720h0m0s" {
		t.Fatalf("storage.telemetry_retention.value = %v, want 720h0m0s", resp.Storage.TelemetryRetention.Value)
	}
	if !resp.Storage.TelemetryRetention.EnvOverride {
		t.Fatal("storage.telemetry_retention.env_override = false, want true")
	}
}

func TestBuildConfigResponseIncludesSerialFields(t *testing.T) {
	cfg := fullTestConfig()
	cfg.Serial.Device = "/dev/ttyUSB0"
	env := config.EnvOverrides{"TRANSPORT_MODE": true, "SERIAL_DTR": true}
	resp := buildConfigResponse(cfg, env, "", time.Time{})

	if resp.TransportMode.Value != config.TransportUDP {
		t.Fatalf("transport_mode.value = %v, want udp", resp.TransportMode.Value)
	}
	if !resp.TransportMode.EnvOverride {
		t.Fatal("transport_mode.env_override = false, want true")
	}
	if resp.Serial.Device.Value != "/dev/ttyUSB0" {
		t.Fatalf("serial.device.value = %v, want /dev/ttyUSB0", resp.Serial.Device.Value)
	}
	if resp.Serial.Baud.Value != 115200 {
		t.Fatalf("serial.baud.value = %v, want 115200", resp.Serial.Baud.Value)
	}
	if !resp.Serial.DTR.EnvOverride {
		t.Fatal("serial.dtr.env_override = false, want true")
	}
	if !resp.Serial.Device.RequiresRestart {
		t.Fatal("serial.device.requires_restart = false, want true")
	}
}

// -------- GET /api/config (HTTP, no auth) --------

func TestGetConfigHTTPReturnsOK(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp configResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// -------- PUT /api/config --------

func TestUpdateConfigPersistsToToml(t *testing.T) {
	dir := t.TempDir()
	cfg := fullTestConfig()
	cfg.MyCall = "QQ0QQ-1"
	cfg.LogLevel = "info"
	server := configTestServer(cfg, config.EnvOverrides{}, dir)

	body, _ := json.Marshal(map[string]any{"log_level": "debug"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// TOML file should exist and contain the updated log_level.
	data, err := os.ReadFile(filepath.Join(dir, "gomeshcomd.toml"))
	if err != nil {
		t.Fatalf("toml file not created: %v", err)
	}
	content := string(data)
	if !contains(content, `log_level = "debug"`) {
		t.Errorf("expected log_level = \"debug\" in TOML; got:\n%s", content)
	}
}

func TestUpdateConfigSerialPersistsToToml(t *testing.T) {
	dir := t.TempDir()
	server := configTestServer(fullTestConfig(), config.EnvOverrides{}, dir)
	body, err := json.Marshal(map[string]any{
		"transport_mode": "serial",
		"serial": map[string]any{
			"device": "/dev/ttyUSB0",
			"dtr":    false,
			"rts":    false,
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if server.cfg.TransportMode != config.TransportSerial {
		t.Fatalf("TransportMode = %q, want serial", server.cfg.TransportMode)
	}
	if server.cfg.Serial.Device != "/dev/ttyUSB0" {
		t.Fatalf("Serial.Device = %q, want /dev/ttyUSB0", server.cfg.Serial.Device)
	}

	data, err := os.ReadFile(filepath.Join(dir, "gomeshcomd.toml"))
	if err != nil {
		t.Fatalf("read TOML: %v", err)
	}
	content := string(data)
	if !contains(content, `transport_mode = "serial"`) {
		t.Errorf("missing transport_mode in TOML:\n%s", content)
	}
	if !contains(content, "[serial]") || !contains(content, `device = "/dev/ttyUSB0"`) {
		t.Errorf("missing serial section in TOML:\n%s", content)
	}
}

func TestUpdateConfigNormalizesSerialValues(t *testing.T) {
	server := configTestServer(fullTestConfig(), config.EnvOverrides{}, "")
	body, err := json.Marshal(map[string]any{
		"transport_mode": " SERIAL ",
		"serial": map[string]any{
			"device":       " /dev/ttyUSB0 ",
			"parity":       " EVEN ",
			"flow_control": " NONE ",
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if server.cfg.TransportMode != config.TransportSerial ||
		server.cfg.Serial.Device != "/dev/ttyUSB0" ||
		server.cfg.Serial.Parity != "even" ||
		server.cfg.Serial.FlowControl != "none" {
		t.Fatalf("config was not normalized: mode=%q serial=%+v", server.cfg.TransportMode, server.cfg.Serial)
	}
}

func TestUpdateConfigStoragePathRequiresRestart(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{"storage": map[string]any{"sqlite_path": "./data/other.db"}})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if server.cfg.Storage.SQLitePath != "./data/other.db" {
		t.Fatalf("Storage.SQLitePath = %q, want ./data/other.db", server.cfg.Storage.SQLitePath)
	}

	var resp struct {
		RequiresRestart bool `json:"requires_restart"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.RequiresRestart {
		t.Fatal("requires_restart = false, want true")
	}
}

func TestUpdateConfigStoragePurgeDurations(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{"storage": map[string]any{
		"purge_interval":        "2h",
		"receive_log_retention": "14d",
		"public_chat_retention": "10d",
		"nodes_retention":       "3d",
		"telemetry_retention":   "21d",
	}})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if server.cfg.Storage.PurgeInterval != 2*time.Hour {
		t.Fatalf("Storage.PurgeInterval = %s, want 2h", server.cfg.Storage.PurgeInterval)
	}
	if server.cfg.Storage.ReceiveLogRetention != 14*24*time.Hour {
		t.Fatalf("Storage.ReceiveLogRetention = %s, want 14d", server.cfg.Storage.ReceiveLogRetention)
	}
	if server.cfg.Storage.PublicChatRetention != 10*24*time.Hour {
		t.Fatalf("Storage.PublicChatRetention = %s, want 10d", server.cfg.Storage.PublicChatRetention)
	}
	if server.cfg.Storage.NodesRetention != 3*24*time.Hour {
		t.Fatalf("Storage.NodesRetention = %s, want 3d", server.cfg.Storage.NodesRetention)
	}
	if server.cfg.Storage.TelemetryRetention != 21*24*time.Hour {
		t.Fatalf("Storage.TelemetryRetention = %s, want 21d", server.cfg.Storage.TelemetryRetention)
	}
}

func TestUpdateConfigInvalidValueRejected(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{"log_level": "trace"}) // invalid
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateConfigEnvLockedFieldRejected(t *testing.T) {
	cfg := fullTestConfig()
	env := config.EnvOverrides{"MY_CALL": true}
	server := configTestServer(cfg, env, "")

	body, _ := json.Marshal(map[string]any{"my_call": "QQ5QQQ-1"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestUpdateConfigPasswordMaskIgnored(t *testing.T) {
	// Verify that sending the mask sentinel does not update the password.
	// Auth is disabled (no username/password) so the endpoint is reachable.
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{
		"auth": map[string]any{"password": passwordMask},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Password must remain empty — mask sentinel must not be stored.
	if server.cfg.Auth.Password != "" {
		t.Errorf("password = %q, want empty (mask sentinel must not update)", server.cfg.Auth.Password)
	}
}

func TestUpdateConfigInvalidDurationRejected(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{"receive_log": map[string]any{"replay_window": "notaduration"}})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateConfigInvalidSerialDurationRejected(t *testing.T) {
	server := configTestServer(fullTestConfig(), config.EnvOverrides{}, "")
	body, err := json.Marshal(map[string]any{
		"serial": map[string]any{"reconnect_initial": "notaduration"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateConfigEnvLockedSerialFieldRejected(t *testing.T) {
	env := config.EnvOverrides{"SERIAL_DEVICE": true}
	server := configTestServer(fullTestConfig(), env, "")
	body, err := json.Marshal(map[string]any{
		"serial": map[string]any{"device": "/dev/ttyUSB0"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
}

func TestApplySerialUpdateAppliesEveryField(t *testing.T) {
	device := "COM7"
	baud := 57600
	dataBits := 7
	parity := "even"
	stopBits := 2
	flowControl := "none"
	dtr := true
	rts := true
	readTimeout := "2s"
	reconnectInitial := "3s"
	reconnectMax := "40s"
	stableResetAfter := "1m"
	maxRecordBytes := 8192
	serial := config.Serial{}

	err := applySerialUpdate(&serial, &configUpdateSerial{
		Device:           &device,
		Baud:             &baud,
		DataBits:         &dataBits,
		Parity:           &parity,
		StopBits:         &stopBits,
		FlowControl:      &flowControl,
		DTR:              &dtr,
		RTS:              &rts,
		ReadTimeout:      &readTimeout,
		ReconnectInitial: &reconnectInitial,
		ReconnectMax:     &reconnectMax,
		StableResetAfter: &stableResetAfter,
		MaxRecordBytes:   &maxRecordBytes,
	})
	if err != nil {
		t.Fatalf("applySerialUpdate() error = %v", err)
	}

	want := config.Serial{
		Device:           "COM7",
		Baud:             57600,
		DataBits:         7,
		Parity:           "even",
		StopBits:         2,
		FlowControl:      "none",
		DTR:              true,
		RTS:              true,
		ReadTimeout:      2 * time.Second,
		ReconnectInitial: 3 * time.Second,
		ReconnectMax:     40 * time.Second,
		StableResetAfter: time.Minute,
		MaxRecordBytes:   8192,
	}
	if serial != want {
		t.Fatalf("serial = %+v, want %+v", serial, want)
	}
}

func TestApplySerialUpdateRejectsInvalidDurations(t *testing.T) {
	invalid := "invalid"
	tests := map[string]configUpdateSerial{
		"read timeout":       {ReadTimeout: &invalid},
		"reconnect initial":  {ReconnectInitial: &invalid},
		"reconnect maximum":  {ReconnectMax: &invalid},
		"stable reset after": {StableResetAfter: &invalid},
	}

	for name, update := range tests {
		t.Run(name, func(t *testing.T) {
			if err := applySerialUpdate(&config.Serial{}, &update); err == nil {
				t.Fatal("applySerialUpdate() error = nil")
			}
		})
	}
}

func TestUpdateConfigInvalidJSONRejected(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUpdateConfigRequiresRestartInResponse(t *testing.T) {
	cfg := fullTestConfig()
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	body, _ := json.Marshal(map[string]any{"http_addr": "127.0.0.1:9090"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		RequiresRestart bool `json:"requires_restart"`
	}
	json.NewDecoder(rec.Body).Decode(&resp) //nolint: errcheck
	if !resp.RequiresRestart {
		t.Error("requires_restart should be true when http_addr changed")
	}
}

func TestUpdateConfigMyCallLiveApply(t *testing.T) {
	cfg := fullTestConfig()
	cfg.MyCall = "QQ0QQ-1"
	bus := events.NewBus()
	ctx := t.Context()
	_ = ctx
	subscriber := bus.Subscribe(ctx)

	server := NewServer(cfg, "v-test", bus, nil, nil, nil, nil, nil, nil,
		WithEnvOverrides(config.EnvOverrides{}),
	)

	body, _ := json.Marshal(map[string]any{"my_call": "QQ5QQQ-1"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	// Effective callsign must be updated in server.
	if server.cfg.MyCall != "QQ5QQQ-1" {
		t.Errorf("server.cfg.MyCall = %q, want QQ5QQQ-1", server.cfg.MyCall)
	}

	// SSE event must have been published.
	select {
	case ev := <-subscriber:
		if ev.Type != "station.identity" {
			t.Errorf("event type = %q, want station.identity", ev.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("no station.identity SSE event published")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
