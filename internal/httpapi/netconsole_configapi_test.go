package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/config"
)

func TestBuildConfigResponseMasksNetConsolePassword(t *testing.T) {
	cfg := fullTestConfig()
	cfg.NetConsole.Address = "meshcom.local:2323"
	cfg.NetConsole.Password = "secret"
	response := buildConfigResponse(cfg, config.EnvOverrides{"NETCONSOLE_PASSWORD": true}, "", time.Time{})

	if response.NetConsole.Password.Value != passwordMask {
		t.Fatalf("password = %v, want mask", response.NetConsole.Password.Value)
	}
	if !response.NetConsole.Password.EnvOverride || !response.NetConsole.Password.RequiresRestart {
		t.Fatalf("password metadata = %+v", response.NetConsole.Password)
	}
	if response.NetConsole.Address.Value != "meshcom.local:2323" {
		t.Fatalf("address = %v", response.NetConsole.Address.Value)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("serialized config response contains NETConsole password")
	}
}

func TestUpdateConfigNetConsolePersistsAndRequiresRestart(t *testing.T) {
	dir := t.TempDir()
	server := configTestServer(fullTestConfig(), config.EnvOverrides{}, dir)
	body, err := json.Marshal(map[string]any{
		"transport_mode": "netconsole",
		"netconsole": map[string]any{
			"address":  "meshcom.local:2323",
			"password": "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if server.cfg.TransportMode != config.TransportNetConsole ||
		server.cfg.NetConsole.Address != "meshcom.local:2323" ||
		server.cfg.NetConsole.Password != "secret" {
		t.Fatal("persisted NETConsole configuration does not match update")
	}
	if !strings.Contains(recorder.Body.String(), `"requires_restart":true`) ||
		strings.Contains(recorder.Body.String(), `"secret"`) {
		t.Fatalf("response = %s", recorder.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(dir, "gomeshcomd.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `password = "secret"`) {
		t.Fatal("TOML does not persist NETConsole password")
	}
}

func TestUpdateConfigNetConsolePasswordMaskPreservesAndEmptyClears(t *testing.T) {
	cfg := fullTestConfig()
	cfg.TransportMode = config.TransportNetConsole
	cfg.NetConsole.Address = "meshcom.local:2323"
	cfg.NetConsole.Password = "secret"
	server := configTestServer(cfg, config.EnvOverrides{}, "")

	for _, password := range []string{passwordMask, ""} {
		body, err := json.Marshal(map[string]any{"netconsole": map[string]any{"password": password}})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("password %q status = %d, body = %s", password, recorder.Code, recorder.Body.String())
		}
		if password == passwordMask && server.cfg.NetConsole.Password != "secret" {
			t.Fatalf("mask changed password to %q", server.cfg.NetConsole.Password)
		}
	}
	if server.cfg.NetConsole.Password != "" {
		t.Fatalf("empty update left password = %q", server.cfg.NetConsole.Password)
	}
}

func TestUpdateConfigNetConsoleEnvironmentLockAndInvalidDuration(t *testing.T) {
	tests := []struct {
		name       string
		env        config.EnvOverrides
		patch      map[string]any
		wantStatus int
	}{
		{
			name:       "environment password blocks clear",
			env:        config.EnvOverrides{"NETCONSOLE_PASSWORD": true},
			patch:      map[string]any{"netconsole": map[string]any{"password": ""}},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "mask is not an environment change",
			env:        config.EnvOverrides{"NETCONSOLE_PASSWORD": true},
			patch:      map[string]any{"netconsole": map[string]any{"password": passwordMask}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid duration",
			patch:      map[string]any{"netconsole": map[string]any{"auth_timeout": "invalid"}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := configTestServer(fullTestConfig(), tt.env, "")
			body, err := json.Marshal(tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}
