package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/config"
)

func TestEnsureDataDirs(t *testing.T) {
	dataDir := t.TempDir()

	if err := ensureDataDirs(dataDir); err != nil {
		t.Fatalf("ensureDataDirs() error = %v", err)
	}

	for _, dir := range []string{"raw", "nodes", "chat", "stats", "configs"} {
		info, err := os.Stat(filepath.Join(dataDir, dir))
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not directory", dir)
		}
	}

	if _, err := os.Stat(filepath.Join(dataDir, "messages")); !os.IsNotExist(err) {
		t.Fatalf("messages stat error = %v, want not exist", err)
	}
}

func TestStartupBanner(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "0.0.0.0:8080",
		UDPListenAddr: "0.0.0.0:1799",
		NodeAddr:      "192.168.0.2:1799",
		MyCall:        "QQ1ABC-1",
	}

	banner := startupBanner(cfg, cfg.MyCall)
	wants := []string{
		"GOMESHCOMD",
		"MeshCom UDP Link Terminal",
		"STATUS   READY",
		"VERSION  dev",
		"MYCALL   QQ1ABC-1",
		"NODE     192.168.0.2:1799",
		"HELP     gomeshcomd --help",
		"UDP RX   0.0.0.0:1799",
		"WEB UI   http://127.0.0.1:8080",
	}

	for _, want := range wants {
		if !strings.Contains(banner, want) {
			t.Fatalf("startup banner missing %q:\n%s", want, banner)
		}
	}
}

func TestStartupBannerAutoDetectNode(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "127.0.0.1:8080",
		UDPListenAddr: "0.0.0.0:1799",
		NodeAddr:      "",
		MyCall:        "QQ1ABC-1",
	}

	banner := startupBanner(cfg, cfg.MyCall)
	if !strings.Contains(banner, "NODE     (auto-detect from incoming UDP)") {
		t.Fatalf("startup banner missing auto-detect message:\n%s", banner)
	}
}

func TestStartupBannerSerialTransport(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "127.0.0.1:8080",
		TransportMode: config.TransportSerial,
		MyCall:        "QQ1ABC-1",
		Serial: config.Serial{
			Device: "/dev/ttyUSB0",
		},
	}

	banner := startupBanner(cfg, cfg.MyCall)
	for _, want := range []string{
		"MeshCom SERIAL Link Terminal",
		"NODE     /dev/ttyUSB0",
		"SERIAL   /dev/ttyUSB0",
	} {
		if !strings.Contains(banner, want) {
			t.Fatalf("startup banner missing %q:\n%s", want, banner)
		}
	}
	if strings.Contains(banner, "UDP RX") {
		t.Fatalf("serial startup banner contains UDP row:\n%s", banner)
	}
}

func TestStartupBannerNetConsoleTransport(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "127.0.0.1:8080",
		TransportMode: config.TransportNetConsole,
		MyCall:        "QQ1ABC-1",
		NetConsole: config.NetConsole{
			Address: "meshcom.local:1799",
		},
	}

	banner := startupBanner(cfg, cfg.MyCall)
	for _, want := range []string{
		"MeshCom NETCONSOLE Link Terminal",
		"NODE     meshcom.local:1799",
		"NETCON   meshcom.local:1799",
	} {
		if !strings.Contains(banner, want) {
			t.Fatalf("startup banner missing %q:\n%s", want, banner)
		}
	}
	if strings.Contains(banner, "UDP RX") {
		t.Fatalf("NETConsole startup banner contains UDP row:\n%s", banner)
	}
}

func TestStartupBannerRowsStayBoxed(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "127.0.0.1:8080",
		UDPListenAddr: "0.0.0.0:1799",
		NodeAddr:      "192.168.0.2:1799",
	}

	rows := strings.Split(strings.TrimSuffix(startupBanner(cfg, cfg.MyCall), "\n"), "\n")
	if len(rows) == 0 {
		t.Fatal("startup banner is empty")
	}

	wantLength := len(rows[0])
	for _, row := range rows {
		if len(row) != wantLength {
			t.Fatalf("row length = %d, want %d: %q", len(row), wantLength, row)
		}
	}
}

func TestStartupBannerShowsUnsetMyCallHint(t *testing.T) {
	cfg := config.Config{
		HTTPAddr:      "127.0.0.1:8080",
		UDPListenAddr: "0.0.0.0:1799",
		NodeAddr:      "192.168.0.2:1799",
	}

	banner := startupBanner(cfg, cfg.MyCall)
	wants := []string{
		"MYCALL   (unset)",
		"DMs      hidden until MyCall set",
		"MSG      node addr required",
	}

	for _, want := range wants {
		if !strings.Contains(banner, want) {
			t.Fatalf("startup banner missing %q:\n%s", want, banner)
		}
	}
}

func TestWebInterfaceURL(t *testing.T) {
	tests := []struct {
		name     string
		httpAddr string
		want     string
	}{
		{
			name:     "loopback",
			httpAddr: "127.0.0.1:8080",
			want:     "http://127.0.0.1:8080",
		},
		{
			name:     "all interfaces",
			httpAddr: "0.0.0.0:8080",
			want:     "http://127.0.0.1:8080",
		},
		{
			name:     "empty host",
			httpAddr: ":8080",
			want:     "http://127.0.0.1:8080",
		},
		{
			name:     "ipv6 all interfaces",
			httpAddr: "[::]:8080",
			want:     "http://127.0.0.1:8080",
		},
		{
			name:     "ipv6 loopback",
			httpAddr: "[::1]:8080",
			want:     "http://[::1]:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webInterfaceURL(tt.httpAddr); got != tt.want {
				t.Fatalf("webInterfaceURL(%q) = %q, want %q", tt.httpAddr, got, tt.want)
			}
		})
	}
}

func TestOpenStorageAndImportImportsLegacyFilesOnce(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	cfg := config.Config{
		DataDir: dataDir,
		MyCall:  "QQ0QQ-1",
		ReceiveLog: config.ReceiveLog{
			Path: filepath.Join(dataDir, "raw"),
		},
		ChatLog: config.ChatLog{
			Path: filepath.Join(dataDir, "chat"),
		},
		Stats: config.Stats{
			Path: filepath.Join(dataDir, "stats", "stats.json"),
		},
		Storage: config.Storage{
			SQLitePath: filepath.Join(dataDir, "gomeshcom.db"),
		},
	}

	writeStartupFile(t, filepath.Join(dataDir, "nodes", "positions.json"), fmt.Sprintf(`{"NODE-1":{"lat":43.7,"lng":10.4,"lastseen":%q}}`, now.Format(time.RFC3339Nano)))
	writeStartupFile(t, filepath.Join(dataDir, "raw", "received.20260701.jsonl"), fmt.Sprintf(`{"received_at":%q,"remote_addr":"127.0.0.1:1799","bytes":1,"raw":"{}"}`+"\n", now.Format(time.RFC3339Nano)))
	writeStartupFile(t, filepath.Join(dataDir, "chat", "P_broadcast.jsonl"), fmt.Sprintf(`{"received_at":%q,"src":"SRC-1","dst":"*","msg":"hello"}`+"\n", now.Format(time.RFC3339Nano)))
	writeStartupFile(t, filepath.Join(dataDir, "chat", "DM_QQ1ABC-1.jsonl"), fmt.Sprintf(`{"received_at":%q,"src":"QQ1ABC-1","dst":"QQ0QQ-1","msg":"legacy dm"}`+"\n", now.Format(time.RFC3339Nano)))
	writeStartupFile(t, filepath.Join(dataDir, "chat", "msg_idx.json"), fmt.Sprintf(`{"P_broadcast":{"lastRead":%q},"DM_QQ1ABC-1":{"lastRead":%q}}`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)))
	writeStartupFile(t, filepath.Join(dataDir, "stats", "dm_stats.json"), `{"QQ1ABC-1":{"sent":2,"ack":1}}`)
	writeStartupFile(t, filepath.Join(dataDir, "stats", "stats.json"), fmt.Sprintf(`{"%d":{"hour":%d,"public":1,"total":1}}`, now.Truncate(time.Hour).Unix(), now.Truncate(time.Hour).Unix()))
	writeStartupFile(t, filepath.Join(dataDir, "channel_show.json"), `{"mode":"allowlist","channels":["*"]}`)
	writeStartupFile(t, filepath.Join(dataDir, "configs", "station.json"), `{"callsign":"QQ9XYZ-1"}`)
	writeStartupFile(t, filepath.Join(dataDir, "http-sessions.json"), fmt.Sprintf(`{"sessions":{"token-hash":%q}}`, now.Add(time.Hour).Format(time.RFC3339Nano)))

	if err := runLegacyDataMigration(cfg); err != nil {
		t.Fatalf("runLegacyDataMigration() error = %v", err)
	}
	db, err := openStorageAndImport(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openStorageAndImport() error = %v", err)
	}
	defer db.Close()

	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM nodes`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM receive_log`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chats_public`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chats_dm`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chat_reads`, 2)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chats_dm WHERE conversation_id = 'DM_QQ9XYZ_QQ1ABC-1'`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chat_reads WHERE conversation_id = 'DM_QQ9XYZ-1_QQ1ABC-1'`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM dm_stats`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM stats_hourly`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM channel_show_channels`, 1)
	if _, err := os.Stat(filepath.Join(dataDir, "configs", "channel_show.json")); err != nil {
		t.Fatalf("legacy channel_show not moved before import: %v", err)
	}
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM station_identity`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM http_sessions`, 1)
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM data_imports`, 9)

	if err := db.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}
	writeStartupFile(t, filepath.Join(dataDir, "chat", "P_broadcast.jsonl"), fmt.Sprintf(`{"received_at":%q,"src":"SRC-2","dst":"*","msg":"second"}`+"\n", now.Add(time.Minute).Format(time.RFC3339Nano)))
	db, err = openStorageAndImport(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second openStorageAndImport() error = %v", err)
	}
	defer db.Close()
	assertStartupCount(t, db.SQL(), `SELECT COUNT(*) FROM chats_public`, 1)
}

func writeStartupFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertStartupCount(t *testing.T, db interface {
	QueryRow(query string, args ...any) *sql.Row
}, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}
