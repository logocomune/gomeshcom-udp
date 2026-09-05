package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validNetConsoleConfig() Config {
	cfg := builtInDefaultConfig()
	cfg.MyCall = "QQ1ABC-1"
	cfg.TransportMode = TransportNetConsole
	cfg.NetConsole.Address = "meshcom.local:2323"
	return cfg
}

func TestBuiltInNetConsoleDefaults(t *testing.T) {
	got := builtInDefaultConfig().NetConsole
	want := NetConsole{
		ConnectTimeout:   5 * time.Second,
		AuthTimeout:      5 * time.Second,
		WriteTimeout:     5 * time.Second,
		ReconnectInitial: time.Second,
		ReconnectMax:     30 * time.Second,
		StableResetAfter: 30 * time.Second,
		MaxAuthLineBytes: 128,
		MaxRecordBytes:   65536,
	}
	if got != want {
		t.Fatalf("NetConsole = %+v, want %+v", redactedNetConsole(got), redactedNetConsole(want))
	}
}

func TestValidateNetConsoleTransport(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*NetConsole)
		wantErr string
	}{
		{name: "DNS address"},
		{name: "IPv4 address", change: func(n *NetConsole) { n.Address = "192.168.1.53:2323" }},
		{name: "IPv6 address", change: func(n *NetConsole) { n.Address = "[2001:db8::1]:2323" }},
		{name: "missing address", change: func(n *NetConsole) { n.Address = "" }, wantErr: "netconsole address"},
		{name: "missing host", change: func(n *NetConsole) { n.Address = ":2323" }, wantErr: "host is required"},
		{name: "missing port", change: func(n *NetConsole) { n.Address = "meshcom.local" }, wantErr: "netconsole address"},
		{name: "invalid port", change: func(n *NetConsole) { n.Address = "meshcom.local:70000" }, wantErr: "port"},
		{name: "password 14 bytes", change: func(n *NetConsole) { n.Password = "12345678901234" }},
		{name: "password too long", change: func(n *NetConsole) { n.Password = "123456789012345" }, wantErr: "14 bytes"},
		{name: "password byte limit", change: func(n *NetConsole) { n.Password = "èèèèèèèè" }, wantErr: "14 bytes"},
		{name: "password newline", change: func(n *NetConsole) { n.Password = "bad\npassword" }, wantErr: "control byte"},
		{name: "connect timeout", change: func(n *NetConsole) { n.ConnectTimeout = 0 }, wantErr: "connect timeout"},
		{name: "auth timeout", change: func(n *NetConsole) { n.AuthTimeout = 0 }, wantErr: "authentication timeout"},
		{name: "write timeout", change: func(n *NetConsole) { n.WriteTimeout = 0 }, wantErr: "write timeout"},
		{name: "reconnect initial", change: func(n *NetConsole) { n.ReconnectInitial = 0 }, wantErr: "reconnect initial"},
		{name: "reconnect maximum", change: func(n *NetConsole) { n.ReconnectMax = 0 }, wantErr: "reconnect maximum"},
		{name: "reconnect ordering", change: func(n *NetConsole) { n.ReconnectInitial = time.Minute }, wantErr: "must not exceed"},
		{name: "stable reset", change: func(n *NetConsole) { n.StableResetAfter = 0 }, wantErr: "stable reset"},
		{name: "auth line too small", change: func(n *NetConsole) { n.MaxAuthLineBytes = 71 }, wantErr: "authentication line"},
		{name: "auth line too large", change: func(n *NetConsole) { n.MaxAuthLineBytes = 4097 }, wantErr: "authentication line"},
		{name: "record limit", change: func(n *NetConsole) { n.MaxRecordBytes = 0 }, wantErr: "record size"},
		{name: "record limit maximum", change: func(n *NetConsole) { n.MaxRecordBytes = MaxConsoleRecordBytes + 1 }, wantErr: "record size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validNetConsoleConfig()
			if tt.change != nil {
				tt.change(&cfg.NetConsole)
			}
			err := Validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNetConsoleTomlRoundTrip(t *testing.T) {
	cfg := validNetConsoleConfig()
	cfg.NetConsole.Password = "secret"
	cfg.NetConsole.AuthTimeout = 7 * time.Second

	path := filepath.Join(t.TempDir(), "gomeshcomd.toml")
	if err := WriteToml(path, cfg); err != nil {
		t.Fatalf("WriteToml() error = %v", err)
	}
	wire, err := loadTomlFile(path)
	if err != nil {
		t.Fatalf("loadTomlFile() error = %v", err)
	}
	loaded := builtInDefaultConfig()
	mergeToml(&loaded, wire, EnvOverrides{})
	loaded = normalize(loaded)

	if loaded.TransportMode != TransportNetConsole || loaded.NetConsole != cfg.NetConsole {
		t.Fatalf("round trip = mode %q netconsole %+v, want mode %q netconsole %+v", loaded.TransportMode, redactedNetConsole(loaded.NetConsole), cfg.TransportMode, redactedNetConsole(cfg.NetConsole))
	}
}

func TestMergeNetConsoleTomlHonorsEnvironment(t *testing.T) {
	cfg := builtInDefaultConfig()
	address := "toml.local:2323"
	password := "toml-secret"
	authTimeout := tomlDuration{Duration: 9 * time.Second}
	wire := &tomlFile{NetConsole: &tomlNetConsole{
		Address:     &address,
		Password:    &password,
		AuthTimeout: &authTimeout,
	}}

	mergeToml(&cfg, wire, EnvOverrides{"NETCONSOLE_PASSWORD": true})

	if cfg.NetConsole.Address != address || cfg.NetConsole.AuthTimeout != 9*time.Second {
		t.Fatalf("NetConsole = %+v", redactedNetConsole(cfg.NetConsole))
	}
	if cfg.NetConsole.Password != "" {
		t.Fatalf("Password = %q, want env/default value", cfg.NetConsole.Password)
	}
}

func TestLoadNetConsoleEnvironment(t *testing.T) {
	loadTestSetup(t)
	dataDir := t.TempDir()
	t.Setenv("GOMESHCOM_DATA_DIR", dataDir)
	t.Setenv("GOMESHCOM_TRANSPORT_MODE", "netconsole")
	t.Setenv("GOMESHCOM_NETCONSOLE_ADDRESS", "127.0.0.1:2323")
	t.Setenv("GOMESHCOM_NETCONSOLE_PASSWORD", "secret")
	t.Setenv("GOMESHCOM_NETCONSOLE_AUTH_TIMEOUT", "7s")

	cfg, env, _, err := Load("test")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TransportMode != TransportNetConsole ||
		cfg.NetConsole.Address != "127.0.0.1:2323" ||
		cfg.NetConsole.Password != "secret" ||
		cfg.NetConsole.AuthTimeout != 7*time.Second {
		t.Fatalf("loaded config = mode %q netconsole %+v", cfg.TransportMode, redactedNetConsole(cfg.NetConsole))
	}
	for _, suffix := range []string{
		"NETCONSOLE_ADDRESS",
		"NETCONSOLE_PASSWORD",
		"NETCONSOLE_AUTH_TIMEOUT",
	} {
		if !env[suffix] {
			t.Fatalf("env[%q] = false", suffix)
		}
	}

	content, err := os.ReadFile(filepath.Join(dataDir, "configs", "gomeshcomd.toml"))
	if err != nil {
		t.Fatalf("read generated TOML: %v", err)
	}
	if strings.Contains(string(content), "secret") {
		t.Fatal("generated default TOML captured environment password")
	}
}

func redactedNetConsole(netConsole NetConsole) NetConsole {
	if netConsole.Password != "" {
		netConsole.Password = "[REDACTED]"
	}
	return netConsole
}
