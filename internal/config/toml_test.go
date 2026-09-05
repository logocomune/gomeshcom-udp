package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -------- loadTomlFile --------

func TestLoadTomlFileMissing(t *testing.T) {
	tf, err := loadTomlFile(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if tf != nil {
		t.Fatal("expected nil for missing file")
	}
}

func TestLoadTomlFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gomeshcomd.toml")
	content := `
my_call = "IU5PMP-1"
log_level = "debug"
max_message_length = 100

[receive_log]
enabled = false
retention_days = 7
replay_window = "30m"

[storage]
purge_interval = "4h"
receive_log_retention = "30d"
public_chat_retention = "30d"
nodes_retention = "7d"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tf, err := loadTomlFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tf == nil {
		t.Fatal("expected non-nil result")
	}
	if tf.MyCall == nil || *tf.MyCall != "IU5PMP-1" {
		t.Errorf("MyCall = %v, want IU5PMP-1", tf.MyCall)
	}
	if tf.LogLevel == nil || *tf.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", tf.LogLevel)
	}
	if tf.MaxMessageLength == nil || *tf.MaxMessageLength != 100 {
		t.Errorf("MaxMessageLength = %v, want 100", tf.MaxMessageLength)
	}
	if tf.ReceiveLog == nil {
		t.Fatal("expected non-nil ReceiveLog section")
	}
	if tf.ReceiveLog.Enabled == nil || *tf.ReceiveLog.Enabled != false {
		t.Errorf("ReceiveLog.Enabled = %v, want false", tf.ReceiveLog.Enabled)
	}
	if tf.ReceiveLog.ReplayWindow == nil || tf.ReceiveLog.ReplayWindow.Duration != 30*time.Minute {
		t.Errorf("ReceiveLog.ReplayWindow = %v, want 30m", tf.ReceiveLog.ReplayWindow)
	}
	if tf.Storage == nil {
		t.Fatal("expected non-nil Storage section")
	}
	if tf.Storage.ReceiveLogRetention == nil || tf.Storage.ReceiveLogRetention.Duration != 30*24*time.Hour {
		t.Errorf("Storage.ReceiveLogRetention = %v, want 30d", tf.Storage.ReceiveLogRetention)
	}
}

func TestLoadTomlFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gomeshcomd.toml")
	if err := os.WriteFile(path, []byte("not = [valid toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadTomlFile(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadTomlFileInvalidDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gomeshcomd.toml")
	if err := os.WriteFile(path, []byte("[receive_log]\nreplay_window = \"notaduration\""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadTomlFile(path)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestTomlDurationAcceptsDaySuffix(t *testing.T) {
	var d tomlDuration
	if err := d.UnmarshalText([]byte("2d")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if d.Duration != 48*time.Hour {
		t.Fatalf("Duration = %s, want 48h", d.Duration)
	}
}

// -------- mergeToml --------

func TestMergeTomlAppliesValuesWhenNoEnv(t *testing.T) {
	cfg := Config{
		MyCall:           "QQ0XX-1",
		MaxMessageLength: 149,
		LogLevel:         "info",
	}
	myCall := "IU5PMP-1"
	level := "debug"
	tf := &tomlFile{
		MyCall:   &myCall,
		LogLevel: &level,
	}
	mergeToml(&cfg, tf, EnvOverrides{})

	if cfg.MyCall != "IU5PMP-1" {
		t.Errorf("MyCall = %q, want IU5PMP-1", cfg.MyCall)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestMergeTomlSkipsEnvOverriddenFields(t *testing.T) {
	cfg := Config{
		MyCall:   "QQ0XX-1",
		LogLevel: "info",
	}
	myCall := "IU5PMP-1"
	level := "debug"
	tf := &tomlFile{
		MyCall:   &myCall,
		LogLevel: &level,
	}
	env := EnvOverrides{
		"MY_CALL":   true,
		"LOG_LEVEL": true,
	}
	mergeToml(&cfg, tf, env)

	// Env wins — TOML values ignored.
	if cfg.MyCall != "QQ0XX-1" {
		t.Errorf("MyCall = %q, want QQ0XX-1 (env should win)", cfg.MyCall)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info (env should win)", cfg.LogLevel)
	}
}

func TestMergeTomlNilFileIsNoOp(t *testing.T) {
	cfg := Config{MyCall: "QQ0XX-1"}
	mergeToml(&cfg, nil, EnvOverrides{})
	if cfg.MyCall != "QQ0XX-1" {
		t.Errorf("MyCall changed unexpectedly")
	}
}

func TestMergeTomlDataDirNeverOverwritten(t *testing.T) {
	cfg := Config{DataDir: "./data"}
	dir := "./other"
	tf := &tomlFile{DataDir: &dir}
	mergeToml(&cfg, tf, EnvOverrides{})
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data (should not be overwritten from TOML)", cfg.DataDir)
	}
}

func TestMergeTomlAppliesStorageWhenNoEnv(t *testing.T) {
	cfg := Config{Storage: Storage{SQLitePath: "./data/gomeshcom.db"}}
	sqlitePath := "./custom/gomeshcom.db"
	purgeInterval := tomlDuration{Duration: 2 * time.Hour}
	nodesRetention := tomlDuration{Duration: 7 * 24 * time.Hour}
	tf := &tomlFile{Storage: &tomlStorage{
		SQLitePath:     &sqlitePath,
		PurgeInterval:  &purgeInterval,
		NodesRetention: &nodesRetention,
	}}

	mergeToml(&cfg, tf, EnvOverrides{})

	if cfg.Storage.SQLitePath != sqlitePath {
		t.Fatalf("Storage.SQLitePath = %q, want %q", cfg.Storage.SQLitePath, sqlitePath)
	}
	if cfg.Storage.PurgeInterval != 2*time.Hour {
		t.Fatalf("Storage.PurgeInterval = %s, want 2h", cfg.Storage.PurgeInterval)
	}
	if cfg.Storage.NodesRetention != 7*24*time.Hour {
		t.Fatalf("Storage.NodesRetention = %s, want 7d", cfg.Storage.NodesRetention)
	}
}

func TestMergeTomlSkipsEnvStorage(t *testing.T) {
	cfg := Config{Storage: Storage{SQLitePath: "./env/gomeshcom.db"}}
	sqlitePath := "./toml/gomeshcom.db"
	tf := &tomlFile{Storage: &tomlStorage{SQLitePath: &sqlitePath}}

	mergeToml(&cfg, tf, EnvOverrides{"STORAGE_SQLITE_PATH": true})

	if cfg.Storage.SQLitePath != "./env/gomeshcom.db" {
		t.Fatalf("Storage.SQLitePath = %q, want env value", cfg.Storage.SQLitePath)
	}
}

// -------- WriteDefaultToml --------

func TestWriteDefaultTomlCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gomeshcomd.toml")

	cfg := Config{
		HTTPAddr:         "127.0.0.1:8080",
		UDPListenAddr:    "0.0.0.0:1799",
		MyCall:           "QQ0XX-1",
		DataDir:          "./data",
		MaxMessageLength: 149,
		LogLevel:         "info",
		ReceiveLog: ReceiveLog{
			Enabled:       true,
			Path:          "./data/raw",
			RetentionDays: 365,
			ReplayWindow:  time.Hour,
		},
		ChatLog: ChatLog{
			Path:             "./data/chat",
			HistoryWindow:    24 * time.Hour,
			MaxHistoryWindow: 720 * time.Hour,
		},
		Stats: Stats{
			Enabled:       true,
			Path:          "./data/stats/stats.json",
			RetentionDays: 30,
		},
		Send:       Send{DedupTTL: 2 * time.Second},
		Auth:       Auth{SessionTTL: 24 * time.Hour, CookieName: "meshcom_session"},
		RequestLog: RequestLog{Enabled: false},
		Storage:    Storage{SQLitePath: "./data/gomeshcom.db"},
	}

	if err := WriteDefaultToml(path, cfg); err != nil {
		t.Fatalf("WriteDefaultToml() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `my_call = "QQ0XX-1"`) {
		t.Errorf("expected my_call in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `http_addr = "127.0.0.1:8080"`) {
		t.Errorf("expected http_addr in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `[receive_log]`) {
		t.Errorf("expected [receive_log] section, got:\n%s", content)
	}
	if !strings.Contains(content, `[storage]`) {
		t.Errorf("expected [storage] section, got:\n%s", content)
	}
	if !strings.Contains(content, `sqlite_path = "./data/gomeshcom.db"`) {
		t.Errorf("expected sqlite_path in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `purge_interval = "4h"`) {
		t.Errorf("expected purge_interval in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `receive_log_retention = "30d"`) {
		t.Errorf("expected receive_log_retention in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `public_chat_retention = "30d"`) {
		t.Errorf("expected public_chat_retention in TOML, got:\n%s", content)
	}
	if !strings.Contains(content, `nodes_retention = "7d"`) {
		t.Errorf("expected nodes_retention in TOML, got:\n%s", content)
	}
	// Password must never appear in the default file.
	if strings.Contains(content, `password = "secret"`) {
		t.Errorf("password must not appear in generated TOML")
	}
}

func TestWriteDefaultTomlDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gomeshcomd.toml")

	if err := os.WriteFile(path, []byte("# existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteDefaultToml(path, Config{MyCall: "IU5PMP-1"}); err != nil {
		t.Fatalf("WriteDefaultToml() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "# existing\n" {
		t.Errorf("existing file was overwritten; got:\n%s", string(data))
	}
}

func TestWriteDefaultTomlDeterministic(t *testing.T) {
	cfg := Config{
		HTTPAddr:         "127.0.0.1:8080",
		UDPListenAddr:    "0.0.0.0:1799",
		MyCall:           "QQ0XX-1",
		DataDir:          "./data",
		MaxMessageLength: 149,
		LogLevel:         "info",
		ReceiveLog: ReceiveLog{
			Enabled:       true,
			Path:          "./data/raw",
			RetentionDays: 365,
			ReplayWindow:  time.Hour,
		},
		ChatLog: ChatLog{
			Path:             "./data/chat",
			HistoryWindow:    24 * time.Hour,
			MaxHistoryWindow: 720 * time.Hour,
		},
		Stats: Stats{
			Enabled:       true,
			Path:          "./data/stats/stats.json",
			RetentionDays: 30,
		},
		Send:       Send{DedupTTL: 2 * time.Second},
		Auth:       Auth{SessionTTL: 24 * time.Hour, CookieName: "meshcom_session"},
		RequestLog: RequestLog{Enabled: false},
		Storage:    Storage{SQLitePath: "./data/gomeshcom.db"},
	}

	a := buildTomlContent(cfg)
	b := buildTomlContent(cfg)
	if a != b {
		t.Errorf("buildTomlContent is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}
}

func TestWriteDefaultTomlPasswordAlwaysEmpty(t *testing.T) {
	cfg := Config{
		HTTPAddr:         "127.0.0.1:8080",
		UDPListenAddr:    "0.0.0.0:1799",
		MyCall:           "QQ0XX-1",
		DataDir:          "./data",
		MaxMessageLength: 149,
		LogLevel:         "info",
		ChatLog:          ChatLog{Path: "./data/chat", HistoryWindow: time.Hour, MaxHistoryWindow: 24 * time.Hour},
		Auth: Auth{
			Username:   "admin",
			Password:   "super-secret", // must NOT appear in output
			SessionTTL: 24 * time.Hour,
			CookieName: "meshcom_session",
		},
	}

	content := buildTomlContent(cfg)
	if strings.Contains(content, "super-secret") {
		t.Error("password leaked into TOML content")
	}
	// The password field should be present but empty.
	if !strings.Contains(content, `password = ""`) {
		t.Errorf("expected empty password field in TOML, got:\n%s", content)
	}
}

// -------- Load integration: TOML + env precedence --------

func loadTestSetup(t *testing.T) {
	t.Helper()
	oldArgs := os.Args
	os.Args = []string{"gomeshcomd"}
	t.Cleanup(func() { os.Args = oldArgs })
}

func TestLoadTomlOverridesDefault(t *testing.T) {
	loadTestSetup(t)
	dir := t.TempDir()
	t.Setenv("GOMESHCOM_DATA_DIR", dir)
	t.Setenv("GOMESHCOM_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("GOMESHCOM_UDP_LISTEN_ADDR", "0.0.0.0:1799")
	t.Setenv("GOMESHCOM_LOG_LEVEL", "info")

	// Write a TOML file that overrides my_call from the default.
	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `my_call = "IU5PMP-3"` + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "gomeshcomd.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, _, err := Load("test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MyCall != "IU5PMP-3" {
		t.Errorf("MyCall = %q, want IU5PMP-3 (from TOML)", cfg.MyCall)
	}
}

func TestLoadEnvOverridesToml(t *testing.T) {
	loadTestSetup(t)
	dir := t.TempDir()
	t.Setenv("GOMESHCOM_DATA_DIR", dir)
	t.Setenv("GOMESHCOM_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("GOMESHCOM_UDP_LISTEN_ADDR", "0.0.0.0:1799")
	t.Setenv("GOMESHCOM_LOG_LEVEL", "info")
	t.Setenv("GOMESHCOM_MY_CALL", "ENV0XX-1") // env should win

	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `my_call = "TOML0XX-1"` + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "gomeshcomd.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, env, _, err := Load("test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MyCall != "ENV0XX-1" {
		t.Errorf("MyCall = %q, want ENV0XX-1 (env should win)", cfg.MyCall)
	}
	if !env["MY_CALL"] {
		t.Error("expected env[MY_CALL] = true")
	}
}

func TestLoadInvalidTomlFails(t *testing.T) {
	loadTestSetup(t)
	dir := t.TempDir()
	t.Setenv("GOMESHCOM_DATA_DIR", dir)

	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "gomeshcomd.toml"), []byte("not = [valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := Load("test")
	if err == nil {
		t.Fatal("expected error for invalid TOML file")
	}
}

func TestLoadMissingTomlWritesDefault(t *testing.T) {
	loadTestSetup(t)
	dir := t.TempDir()
	t.Setenv("GOMESHCOM_DATA_DIR", dir)
	t.Setenv("GOMESHCOM_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("GOMESHCOM_UDP_LISTEN_ADDR", "0.0.0.0:1799")
	t.Setenv("GOMESHCOM_MY_CALL", "QQ1ABC-1")
	t.Setenv("GOMESHCOM_LOG_LEVEL", "info")

	tomlPath := filepath.Join(dir, "configs", "gomeshcomd.toml")

	if _, err := os.Stat(tomlPath); !os.IsNotExist(err) {
		t.Fatal("expected TOML file to not exist yet")
	}

	_, _, _, err := Load("test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	data, readErr := os.ReadFile(tomlPath)
	if readErr != nil {
		t.Fatalf("expected TOML file to be created on first Load: %v", readErr)
	}

	// File must contain built-in defaults, not the env-overridden callsign.
	content := string(data)
	if strings.Contains(content, "QQ1ABC-1") {
		t.Errorf("generated TOML must not contain env-overridden my_call %q; got:\n%s", "QQ1ABC-1", content)
	}
	if !strings.Contains(content, "QQ0XX-1") {
		t.Errorf("generated TOML must contain built-in default my_call QQ0XX-1; got:\n%s", content)
	}
}

// -------- FuzzLoadTomlFile --------

func FuzzLoadTomlFile(f *testing.F) {
	seeds := []string{
		`my_call = "IU5PMP-1"`,
		`send_delay = "40s"`,
		`[receive_log]
enabled = true
retention_days = 365`,
		`transport_mode = "serial"
[serial]
device = "/dev/ttyUSB0"
baud = 115200
dtr = false
rts = false`,
		`transport_mode = "netconsole"
[netconsole]
address = "meshcom.local:2323"
password = "secret"
connect_timeout = "5s"
auth_timeout = "5s"
write_timeout = "5s"
reconnect_initial = "1s"
reconnect_max = "30s"
stable_reset_after = "30s"
max_auth_line_bytes = 128
max_record_bytes = 65536`,
		`invalid = [`,
		``,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "fuzz.toml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Skip("cannot write fuzz file")
		}
		// Must never panic — errors are acceptable.
		_, _ = loadTomlFile(path)
	})
}
