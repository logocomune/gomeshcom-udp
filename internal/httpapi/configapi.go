package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/config"
	"github.com/logocomune/gomeshcom-client/internal/events"
)

// -------- Response DTO --------

// configFieldMeta wraps a single config value with metadata for the UI.
type configFieldMeta struct {
	Value           any  `json:"value"`
	EnvOverride     bool `json:"env_override"`
	RequiresRestart bool `json:"requires_restart"`
}

// serverInfo holds runtime-only fields that are not config but are useful alongside it.
type serverInfo struct {
	Version       string `json:"version"`
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// configResponse is the shape returned by GET /api/config.
// auth.password value is always masked — never the real value.
type configResponse struct {
	Server           serverInfo               `json:"server"`
	HTTPAddr         configFieldMeta          `json:"http_addr"`
	TransportMode    configFieldMeta          `json:"transport_mode"`
	UDPListenAddr    configFieldMeta          `json:"udp_listen_addr"`
	NodeAddr         configFieldMeta          `json:"node_addr"`
	MyCall           configFieldMeta          `json:"my_call"`
	DataDir          configFieldMeta          `json:"data_dir"`
	MaxMessageLength configFieldMeta          `json:"max_message_length"`
	LogLevel         configFieldMeta          `json:"log_level"`
	ReceiveLog       configReceiveLogResponse `json:"receive_log"`
	Stats            configStatsResponse      `json:"stats"`
	ChatLog          configChatLogResponse    `json:"chat_log"`
	Send             configSendResponse       `json:"send"`
	Forward          configForwardResponse    `json:"forward"`
	Auth             configAuthResponse       `json:"auth"`
	RequestLog       configRequestLogResponse `json:"request_log"`
	Storage          configStorageResponse    `json:"storage"`
	Serial           configSerialResponse     `json:"serial"`
	NetConsole       configNetConsoleResponse `json:"netconsole"`
}

type configSerialResponse struct {
	Device           configFieldMeta `json:"device"`
	Baud             configFieldMeta `json:"baud"`
	DataBits         configFieldMeta `json:"data_bits"`
	Parity           configFieldMeta `json:"parity"`
	StopBits         configFieldMeta `json:"stop_bits"`
	FlowControl      configFieldMeta `json:"flow_control"`
	DTR              configFieldMeta `json:"dtr"`
	RTS              configFieldMeta `json:"rts"`
	ReadTimeout      configFieldMeta `json:"read_timeout"`
	ReconnectInitial configFieldMeta `json:"reconnect_initial"`
	ReconnectMax     configFieldMeta `json:"reconnect_max"`
	StableResetAfter configFieldMeta `json:"stable_reset_after"`
	MaxRecordBytes   configFieldMeta `json:"max_record_bytes"`
}

type configNetConsoleResponse struct {
	Address          configFieldMeta `json:"address"`
	Password         configFieldMeta `json:"password"`
	ConnectTimeout   configFieldMeta `json:"connect_timeout"`
	AuthTimeout      configFieldMeta `json:"auth_timeout"`
	WriteTimeout     configFieldMeta `json:"write_timeout"`
	ReconnectInitial configFieldMeta `json:"reconnect_initial"`
	ReconnectMax     configFieldMeta `json:"reconnect_max"`
	StableResetAfter configFieldMeta `json:"stable_reset_after"`
	MaxAuthLineBytes configFieldMeta `json:"max_auth_line_bytes"`
	MaxRecordBytes   configFieldMeta `json:"max_record_bytes"`
}

type configReceiveLogResponse struct {
	Enabled       configFieldMeta `json:"enabled"`
	Path          configFieldMeta `json:"path"`
	RetentionDays configFieldMeta `json:"retention_days"`
	ReplayWindow  configFieldMeta `json:"replay_window"`
}

type configStatsResponse struct {
	Enabled       configFieldMeta `json:"enabled"`
	Path          configFieldMeta `json:"path"`
	RetentionDays configFieldMeta `json:"retention_days"`
}

type configChatLogResponse struct {
	Path             configFieldMeta `json:"path"`
	HistoryWindow    configFieldMeta `json:"history_window"`
	MaxHistoryWindow configFieldMeta `json:"max_history_window"`
}

type configSendResponse struct {
	DedupTTL configFieldMeta `json:"dedup_ttl"`
}

type configForwardResponse struct {
	Targets configFieldMeta `json:"targets"`
}

type configAuthResponse struct {
	Username   configFieldMeta `json:"username"`
	Password   configFieldMeta `json:"password"` // value is always masked
	SessionTTL configFieldMeta `json:"session_ttl"`
	CookieName configFieldMeta `json:"cookie_name"`
}

type configRequestLogResponse struct {
	Enabled configFieldMeta `json:"enabled"`
}

type configStorageResponse struct {
	SQLitePath          configFieldMeta `json:"sqlite_path"`
	PurgeInterval       configFieldMeta `json:"purge_interval"`
	ReceiveLogRetention configFieldMeta `json:"receive_log_retention"`
	PublicChatRetention configFieldMeta `json:"public_chat_retention"`
	NodesRetention      configFieldMeta `json:"nodes_retention"`
	TelemetryRetention  configFieldMeta `json:"telemetry_retention"`
}

// -------- Update request DTO --------

// configUpdateRequest accepts a partial config update.
// Nil pointer = field not changing. All durations are strings (e.g. "40s").
type configUpdateRequest struct {
	HTTPAddr         *string `json:"http_addr,omitempty"`
	TransportMode    *string `json:"transport_mode,omitempty"`
	UDPListenAddr    *string `json:"udp_listen_addr,omitempty"`
	NodeAddr         *string `json:"node_addr,omitempty"`
	MyCall           *string `json:"my_call,omitempty"`
	MaxMessageLength *int    `json:"max_message_length,omitempty"`
	LogLevel         *string `json:"log_level,omitempty"`

	ReceiveLog *configUpdateReceiveLog `json:"receive_log,omitempty"`
	Stats      *configUpdateStats      `json:"stats,omitempty"`
	ChatLog    *configUpdateChatLog    `json:"chat_log,omitempty"`
	Send       *configUpdateSend       `json:"send,omitempty"`
	Forward    *configUpdateForward    `json:"forward,omitempty"`
	Auth       *configUpdateAuth       `json:"auth,omitempty"`
	RequestLog *configUpdateRequestLog `json:"request_log,omitempty"`
	Storage    *configUpdateStorage    `json:"storage,omitempty"`
	Serial     *configUpdateSerial     `json:"serial,omitempty"`
	NetConsole *configUpdateNetConsole `json:"netconsole,omitempty"`
}

type configUpdateSerial struct {
	Device           *string `json:"device,omitempty"`
	Baud             *int    `json:"baud,omitempty"`
	DataBits         *int    `json:"data_bits,omitempty"`
	Parity           *string `json:"parity,omitempty"`
	StopBits         *int    `json:"stop_bits,omitempty"`
	FlowControl      *string `json:"flow_control,omitempty"`
	DTR              *bool   `json:"dtr,omitempty"`
	RTS              *bool   `json:"rts,omitempty"`
	ReadTimeout      *string `json:"read_timeout,omitempty"`
	ReconnectInitial *string `json:"reconnect_initial,omitempty"`
	ReconnectMax     *string `json:"reconnect_max,omitempty"`
	StableResetAfter *string `json:"stable_reset_after,omitempty"`
	MaxRecordBytes   *int    `json:"max_record_bytes,omitempty"`
}

type configUpdateNetConsole struct {
	Address          *string `json:"address,omitempty"`
	Password         *string `json:"password,omitempty"`
	ConnectTimeout   *string `json:"connect_timeout,omitempty"`
	AuthTimeout      *string `json:"auth_timeout,omitempty"`
	WriteTimeout     *string `json:"write_timeout,omitempty"`
	ReconnectInitial *string `json:"reconnect_initial,omitempty"`
	ReconnectMax     *string `json:"reconnect_max,omitempty"`
	StableResetAfter *string `json:"stable_reset_after,omitempty"`
	MaxAuthLineBytes *int    `json:"max_auth_line_bytes,omitempty"`
	MaxRecordBytes   *int    `json:"max_record_bytes,omitempty"`
}

type configUpdateReceiveLog struct {
	Enabled       *bool   `json:"enabled,omitempty"`
	Path          *string `json:"path,omitempty"`
	RetentionDays *int    `json:"retention_days,omitempty"`
	ReplayWindow  *string `json:"replay_window,omitempty"`
}

type configUpdateStats struct {
	Enabled       *bool   `json:"enabled,omitempty"`
	Path          *string `json:"path,omitempty"`
	RetentionDays *int    `json:"retention_days,omitempty"`
}

type configUpdateChatLog struct {
	Path             *string `json:"path,omitempty"`
	HistoryWindow    *string `json:"history_window,omitempty"`
	MaxHistoryWindow *string `json:"max_history_window,omitempty"`
}

type configUpdateSend struct {
	DedupTTL *string `json:"dedup_ttl,omitempty"`
}

type configUpdateForward struct {
	Targets *string `json:"targets,omitempty"`
}

type configUpdateAuth struct {
	Username   *string `json:"username,omitempty"`
	Password   *string `json:"password,omitempty"`
	SessionTTL *string `json:"session_ttl,omitempty"`
	CookieName *string `json:"cookie_name,omitempty"`
}

type configUpdateRequestLog struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type configUpdateStorage struct {
	SQLitePath          *string `json:"sqlite_path,omitempty"`
	PurgeInterval       *string `json:"purge_interval,omitempty"`
	ReceiveLogRetention *string `json:"receive_log_retention,omitempty"`
	PublicChatRetention *string `json:"public_chat_retention,omitempty"`
	NodesRetention      *string `json:"nodes_retention,omitempty"`
	TelemetryRetention  *string `json:"telemetry_retention,omitempty"`
}

// passwordMask is the sentinel value returned for the password field in GET responses.
const passwordMask = "****"

// -------- Helpers --------

func field(value any, envKey string, env config.EnvOverrides, restart bool) configFieldMeta {
	return configFieldMeta{
		Value:           value,
		EnvOverride:     env[envKey],
		RequiresRestart: restart,
	}
}

func durationField(d time.Duration, envKey string, env config.EnvOverrides, restart bool) configFieldMeta {
	return field(d.String(), envKey, env, restart)
}

func buildConfigResponse(cfg config.Config, env config.EnvOverrides, version string, startedAt time.Time) configResponse {
	// auth.password: show mask if set, empty otherwise — never the real value.
	passwordDisplay := ""
	if cfg.Auth.Password != "" {
		passwordDisplay = passwordMask
	}
	netConsolePasswordDisplay := ""
	if cfg.NetConsole.Password != "" {
		netConsolePasswordDisplay = passwordMask
	}

	now := time.Now().UTC()
	uptimeSecs := int64(0)
	if !startedAt.IsZero() {
		uptimeSecs = int64(now.Sub(startedAt).Seconds())
	}

	return configResponse{
		Server: serverInfo{
			Version:       version,
			StartedAt:     startedAt.Format(time.RFC3339),
			UptimeSeconds: uptimeSecs,
		},
		HTTPAddr:         field(cfg.HTTPAddr, "HTTP_ADDR", env, true),
		TransportMode:    field(cfg.TransportMode, "TRANSPORT_MODE", env, true),
		UDPListenAddr:    field(cfg.UDPListenAddr, "UDP_LISTEN_ADDR", env, true),
		NodeAddr:         field(cfg.NodeAddr, "NODE_ADDR", env, true),
		MyCall:           field(cfg.MyCall, "MY_CALL", env, false),
		DataDir:          field(cfg.DataDir, "DATA_DIR", env, true),
		MaxMessageLength: field(cfg.MaxMessageLength, "MAX_MESSAGE_LENGTH", env, true),
		LogLevel:         field(cfg.LogLevel, "LOG_LEVEL", env, false),
		ReceiveLog: configReceiveLogResponse{
			Enabled:       field(cfg.ReceiveLog.Enabled, "RECEIVE_LOG_ENABLED", env, true),
			Path:          field(cfg.ReceiveLog.Path, "RECEIVE_LOG_PATH", env, true),
			RetentionDays: field(cfg.ReceiveLog.RetentionDays, "RECEIVE_LOG_RETENTION_DAYS", env, true),
			ReplayWindow:  durationField(cfg.ReceiveLog.ReplayWindow, "RECEIVE_LOG_REPLAY_WINDOW", env, true),
		},
		Stats: configStatsResponse{
			Enabled:       field(cfg.Stats.Enabled, "STATS_ENABLED", env, true),
			Path:          field(cfg.Stats.Path, "STATS_PATH", env, true),
			RetentionDays: field(cfg.Stats.RetentionDays, "STATS_RETENTION_DAYS", env, true),
		},
		ChatLog: configChatLogResponse{
			Path:             field(cfg.ChatLog.Path, "CHAT_LOG_PATH", env, true),
			HistoryWindow:    durationField(cfg.ChatLog.HistoryWindow, "CHAT_LOG_HISTORY_WINDOW", env, false),
			MaxHistoryWindow: durationField(cfg.ChatLog.MaxHistoryWindow, "CHAT_LOG_MAX_HISTORY_WINDOW", env, false),
		},
		Send: configSendResponse{
			DedupTTL: durationField(cfg.Send.DedupTTL, "SEND_DEDUP_TTL", env, false),
		},
		Forward: configForwardResponse{
			Targets: field(cfg.Forward.Targets, "FORWARD_TARGETS", env, true),
		},
		Auth: configAuthResponse{
			Username:   field(cfg.Auth.Username, "AUTH_USERNAME", env, true),
			Password:   field(passwordDisplay, "AUTH_PASSWORD", env, true),
			SessionTTL: durationField(cfg.Auth.SessionTTL, "AUTH_SESSION_TTL", env, true),
			CookieName: field(cfg.Auth.CookieName, "AUTH_COOKIE_NAME", env, true),
		},
		RequestLog: configRequestLogResponse{
			Enabled: field(cfg.RequestLog.Enabled, "REQUEST_LOG_ENABLED", env, false),
		},
		Storage: configStorageResponse{
			SQLitePath:          field(cfg.Storage.SQLitePath, "STORAGE_SQLITE_PATH", env, true),
			PurgeInterval:       durationField(cfg.Storage.PurgeInterval, "STORAGE_PURGE_INTERVAL", env, true),
			ReceiveLogRetention: durationField(cfg.Storage.ReceiveLogRetention, "STORAGE_RECEIVE_LOG_RETENTION", env, true),
			PublicChatRetention: durationField(cfg.Storage.PublicChatRetention, "STORAGE_PUBLIC_CHAT_RETENTION", env, true),
			NodesRetention:      durationField(cfg.Storage.NodesRetention, "STORAGE_NODES_RETENTION", env, true),
			TelemetryRetention:  durationField(cfg.Storage.TelemetryRetention, "STORAGE_TELEMETRY_RETENTION", env, true),
		},
		Serial: configSerialResponse{
			Device:           field(cfg.Serial.Device, "SERIAL_DEVICE", env, true),
			Baud:             field(cfg.Serial.Baud, "SERIAL_BAUD", env, true),
			DataBits:         field(cfg.Serial.DataBits, "SERIAL_DATA_BITS", env, true),
			Parity:           field(cfg.Serial.Parity, "SERIAL_PARITY", env, true),
			StopBits:         field(cfg.Serial.StopBits, "SERIAL_STOP_BITS", env, true),
			FlowControl:      field(cfg.Serial.FlowControl, "SERIAL_FLOW_CONTROL", env, true),
			DTR:              field(cfg.Serial.DTR, "SERIAL_DTR", env, true),
			RTS:              field(cfg.Serial.RTS, "SERIAL_RTS", env, true),
			ReadTimeout:      durationField(cfg.Serial.ReadTimeout, "SERIAL_READ_TIMEOUT", env, true),
			ReconnectInitial: durationField(cfg.Serial.ReconnectInitial, "SERIAL_RECONNECT_INITIAL", env, true),
			ReconnectMax:     durationField(cfg.Serial.ReconnectMax, "SERIAL_RECONNECT_MAX", env, true),
			StableResetAfter: durationField(cfg.Serial.StableResetAfter, "SERIAL_STABLE_RESET_AFTER", env, true),
			MaxRecordBytes:   field(cfg.Serial.MaxRecordBytes, "SERIAL_MAX_RECORD_BYTES", env, true),
		},
		NetConsole: configNetConsoleResponse{
			Address:          field(cfg.NetConsole.Address, "NETCONSOLE_ADDRESS", env, true),
			Password:         field(netConsolePasswordDisplay, "NETCONSOLE_PASSWORD", env, true),
			ConnectTimeout:   durationField(cfg.NetConsole.ConnectTimeout, "NETCONSOLE_CONNECT_TIMEOUT", env, true),
			AuthTimeout:      durationField(cfg.NetConsole.AuthTimeout, "NETCONSOLE_AUTH_TIMEOUT", env, true),
			WriteTimeout:     durationField(cfg.NetConsole.WriteTimeout, "NETCONSOLE_WRITE_TIMEOUT", env, true),
			ReconnectInitial: durationField(cfg.NetConsole.ReconnectInitial, "NETCONSOLE_RECONNECT_INITIAL", env, true),
			ReconnectMax:     durationField(cfg.NetConsole.ReconnectMax, "NETCONSOLE_RECONNECT_MAX", env, true),
			StableResetAfter: durationField(cfg.NetConsole.StableResetAfter, "NETCONSOLE_STABLE_RESET_AFTER", env, true),
			MaxAuthLineBytes: field(cfg.NetConsole.MaxAuthLineBytes, "NETCONSOLE_MAX_AUTH_LINE_BYTES", env, true),
			MaxRecordBytes:   field(cfg.NetConsole.MaxRecordBytes, "NETCONSOLE_MAX_RECORD_BYTES", env, true),
		},
	}
}

// -------- Handlers --------

// getConfig handles GET /api/config.
func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.DemoMode {
		writeError(w, http.StatusForbidden, "config API disabled in demo mode")
		return
	}
	writeJSON(w, http.StatusOK, buildConfigResponse(s.cfg, s.envOverrides, s.version, s.startedAt))
}

// updateConfig handles PUT /api/config.
func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DemoMode {
		writeError(w, http.StatusForbidden, "config API disabled in demo mode")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<14) // 16 KB

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	var req configUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// Build candidate config by merging the request onto the current effective config.
	candidate := s.cfg

	// Reject attempts to override env-managed fields.
	if err := checkEnvLocked(req, s.envOverrides); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	requiresRestart := false
	myCallChanged := false

	// Apply top-level fields.
	if req.HTTPAddr != nil {
		candidate.HTTPAddr = *req.HTTPAddr
		requiresRestart = true
	}
	if req.TransportMode != nil {
		candidate.TransportMode = *req.TransportMode
		requiresRestart = true
	}
	if req.UDPListenAddr != nil {
		candidate.UDPListenAddr = *req.UDPListenAddr
		requiresRestart = true
	}
	if req.NodeAddr != nil {
		candidate.NodeAddr = *req.NodeAddr
		requiresRestart = true
	}
	if req.MyCall != nil {
		candidate.MyCall = *req.MyCall
		myCallChanged = true
	}
	if req.MaxMessageLength != nil {
		candidate.MaxMessageLength = *req.MaxMessageLength
		requiresRestart = true
	}
	if req.LogLevel != nil {
		candidate.LogLevel = *req.LogLevel
	}

	// Sections.
	if req.ReceiveLog != nil {
		rl := req.ReceiveLog
		if rl.Enabled != nil {
			candidate.ReceiveLog.Enabled = *rl.Enabled
			requiresRestart = true
		}
		if rl.Path != nil {
			candidate.ReceiveLog.Path = *rl.Path
			requiresRestart = true
		}
		if rl.RetentionDays != nil {
			candidate.ReceiveLog.RetentionDays = *rl.RetentionDays
			requiresRestart = true
		}
		if rl.ReplayWindow != nil {
			d, err := time.ParseDuration(*rl.ReplayWindow)
			if err != nil {
				writeError(w, http.StatusBadRequest, "receive_log.replay_window: "+err.Error())
				return
			}
			candidate.ReceiveLog.ReplayWindow = d
			requiresRestart = true
		}
	}
	if req.Stats != nil {
		s2 := req.Stats
		if s2.Enabled != nil {
			candidate.Stats.Enabled = *s2.Enabled
			requiresRestart = true
		}
		if s2.Path != nil {
			candidate.Stats.Path = *s2.Path
			requiresRestart = true
		}
		if s2.RetentionDays != nil {
			candidate.Stats.RetentionDays = *s2.RetentionDays
			requiresRestart = true
		}
	}
	if req.ChatLog != nil {
		cl := req.ChatLog
		if cl.Path != nil {
			candidate.ChatLog.Path = *cl.Path
			requiresRestart = true
		}
		if cl.HistoryWindow != nil {
			d, err := time.ParseDuration(*cl.HistoryWindow)
			if err != nil {
				writeError(w, http.StatusBadRequest, "chat_log.history_window: "+err.Error())
				return
			}
			candidate.ChatLog.HistoryWindow = d
		}
		if cl.MaxHistoryWindow != nil {
			d, err := time.ParseDuration(*cl.MaxHistoryWindow)
			if err != nil {
				writeError(w, http.StatusBadRequest, "chat_log.max_history_window: "+err.Error())
				return
			}
			candidate.ChatLog.MaxHistoryWindow = d
		}
	}
	if req.Send != nil {
		snd := req.Send
		if snd.DedupTTL != nil {
			d, err := time.ParseDuration(*snd.DedupTTL)
			if err != nil {
				writeError(w, http.StatusBadRequest, "send.dedup_ttl: "+err.Error())
				return
			}
			candidate.Send.DedupTTL = d
		}
	}
	if req.Forward != nil {
		if req.Forward.Targets != nil {
			candidate.Forward.Targets = *req.Forward.Targets
			requiresRestart = true
		}
	}
	if req.Auth != nil {
		a := req.Auth
		if a.Username != nil {
			candidate.Auth.Username = *a.Username
			requiresRestart = true
		}
		// Password: only apply when non-empty and not the mask sentinel.
		if a.Password != nil && *a.Password != "" && *a.Password != passwordMask {
			candidate.Auth.Password = *a.Password
			requiresRestart = true
		}
		if a.SessionTTL != nil {
			d, err := time.ParseDuration(*a.SessionTTL)
			if err != nil {
				writeError(w, http.StatusBadRequest, "auth.session_ttl: "+err.Error())
				return
			}
			candidate.Auth.SessionTTL = d
			requiresRestart = true
		}
		if a.CookieName != nil {
			candidate.Auth.CookieName = *a.CookieName
			requiresRestart = true
		}
	}
	if req.RequestLog != nil {
		if req.RequestLog.Enabled != nil {
			candidate.RequestLog.Enabled = *req.RequestLog.Enabled
		}
	}
	if req.Storage != nil {
		st := req.Storage
		if st.SQLitePath != nil {
			candidate.Storage.SQLitePath = *req.Storage.SQLitePath
			requiresRestart = true
		}
		if st.PurgeInterval != nil {
			d, err := config.ParseDuration(*st.PurgeInterval)
			if err != nil {
				writeError(w, http.StatusBadRequest, "storage.purge_interval: "+err.Error())
				return
			}
			candidate.Storage.PurgeInterval = d
			requiresRestart = true
		}
		if st.ReceiveLogRetention != nil {
			d, err := config.ParseDuration(*st.ReceiveLogRetention)
			if err != nil {
				writeError(w, http.StatusBadRequest, "storage.receive_log_retention: "+err.Error())
				return
			}
			candidate.Storage.ReceiveLogRetention = d
			requiresRestart = true
		}
		if st.PublicChatRetention != nil {
			d, err := config.ParseDuration(*st.PublicChatRetention)
			if err != nil {
				writeError(w, http.StatusBadRequest, "storage.public_chat_retention: "+err.Error())
				return
			}
			candidate.Storage.PublicChatRetention = d
			requiresRestart = true
		}
		if st.NodesRetention != nil {
			d, err := config.ParseDuration(*st.NodesRetention)
			if err != nil {
				writeError(w, http.StatusBadRequest, "storage.nodes_retention: "+err.Error())
				return
			}
			candidate.Storage.NodesRetention = d
			requiresRestart = true
		}
		if st.TelemetryRetention != nil {
			d, err := config.ParseDuration(*st.TelemetryRetention)
			if err != nil {
				writeError(w, http.StatusBadRequest, "storage.telemetry_retention: "+err.Error())
				return
			}
			candidate.Storage.TelemetryRetention = d
			requiresRestart = true
		}
	}
	if req.Serial != nil {
		if err := applySerialUpdate(&candidate.Serial, req.Serial); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		requiresRestart = true
	}
	if req.NetConsole != nil {
		if err := applyNetConsoleUpdate(&candidate.NetConsole, req.NetConsole); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		requiresRestart = true
	}

	// Normalize and validate before persisting.
	candidate = config.Normalize(candidate)
	if err := config.Validate(candidate); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Persist to TOML atomically.
	if s.tomlPath != "" {
		if err := config.WriteToml(s.tomlPath, candidate); err != nil {
			slog.Error("config save failed", "error", err)
			writeError(w, http.StatusInternalServerError, "persist config failed")
			return
		}
	}

	// Apply live-applyable fields.
	if myCallChanged && s.identity != nil {
		accepted, err := s.identity.Update(candidate.MyCall)
		if err != nil {
			// Validation passed above, but identity.Update may normalize differently.
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		candidate.MyCall = accepted

		if saveErr := s.identity.SaveIfDirty(); saveErr != nil {
			slog.Error("station identity save failed after config update", "error", saveErr)
		}

		if s.bus != nil {
			fwdTargets, _ := config.ParseForwardTargets(candidate.Forward.Targets)
			s.bus.Publish(events.Event{
				Type: "station.identity",
				Data: stationIdentityEvent{
					Callsign:           accepted,
					Version:            s.version,
					TxDisabled:         candidate.DemoMode,
					ForwardTargetCount: len(fwdTargets),
				},
			})
		}
	}

	// Update the server's in-memory config.
	s.cfg = candidate

	type updateResponse struct {
		Config          configResponse `json:"config"`
		RequiresRestart bool           `json:"requires_restart"`
	}
	writeJSON(w, http.StatusOK, updateResponse{
		Config:          buildConfigResponse(s.cfg, s.envOverrides, s.version, s.startedAt),
		RequiresRestart: requiresRestart,
	})
}

func applySerialUpdate(serial *config.Serial, update *configUpdateSerial) error {
	if update.Device != nil {
		serial.Device = *update.Device
	}
	if update.Baud != nil {
		serial.Baud = *update.Baud
	}
	if update.DataBits != nil {
		serial.DataBits = *update.DataBits
	}
	if update.Parity != nil {
		serial.Parity = *update.Parity
	}
	if update.StopBits != nil {
		serial.StopBits = *update.StopBits
	}
	if update.FlowControl != nil {
		serial.FlowControl = *update.FlowControl
	}
	if update.DTR != nil {
		serial.DTR = *update.DTR
	}
	if update.RTS != nil {
		serial.RTS = *update.RTS
	}
	if update.ReadTimeout != nil {
		duration, err := config.ParseDuration(*update.ReadTimeout)
		if err != nil {
			return fmt.Errorf("serial.read_timeout: %w", err)
		}
		serial.ReadTimeout = duration
	}
	if update.ReconnectInitial != nil {
		duration, err := config.ParseDuration(*update.ReconnectInitial)
		if err != nil {
			return fmt.Errorf("serial.reconnect_initial: %w", err)
		}
		serial.ReconnectInitial = duration
	}
	if update.ReconnectMax != nil {
		duration, err := config.ParseDuration(*update.ReconnectMax)
		if err != nil {
			return fmt.Errorf("serial.reconnect_max: %w", err)
		}
		serial.ReconnectMax = duration
	}
	if update.StableResetAfter != nil {
		duration, err := config.ParseDuration(*update.StableResetAfter)
		if err != nil {
			return fmt.Errorf("serial.stable_reset_after: %w", err)
		}
		serial.StableResetAfter = duration
	}
	if update.MaxRecordBytes != nil {
		serial.MaxRecordBytes = *update.MaxRecordBytes
	}
	return nil
}

func applyNetConsoleUpdate(target *config.NetConsole, update *configUpdateNetConsole) error {
	if update.Address != nil {
		target.Address = *update.Address
	}
	if update.Password != nil && *update.Password != passwordMask {
		target.Password = *update.Password
	}
	durationUpdates := []struct {
		value  *string
		target *time.Duration
		name   string
	}{
		{update.ConnectTimeout, &target.ConnectTimeout, "connect_timeout"},
		{update.AuthTimeout, &target.AuthTimeout, "auth_timeout"},
		{update.WriteTimeout, &target.WriteTimeout, "write_timeout"},
		{update.ReconnectInitial, &target.ReconnectInitial, "reconnect_initial"},
		{update.ReconnectMax, &target.ReconnectMax, "reconnect_max"},
		{update.StableResetAfter, &target.StableResetAfter, "stable_reset_after"},
	}
	for _, durationUpdate := range durationUpdates {
		if durationUpdate.value == nil {
			continue
		}
		duration, err := config.ParseDuration(*durationUpdate.value)
		if err != nil {
			return fmt.Errorf("netconsole.%s: %w", durationUpdate.name, err)
		}
		*durationUpdate.target = duration
	}
	if update.MaxAuthLineBytes != nil {
		target.MaxAuthLineBytes = *update.MaxAuthLineBytes
	}
	if update.MaxRecordBytes != nil {
		target.MaxRecordBytes = *update.MaxRecordBytes
	}
	return nil
}

// checkEnvLocked returns an error if the update attempts to change any field
// that is currently managed by an environment variable.
func checkEnvLocked(req configUpdateRequest, env config.EnvOverrides) error {
	type check struct {
		present bool
		suffix  string
	}

	checks := []check{
		{req.HTTPAddr != nil, "HTTP_ADDR"},
		{req.TransportMode != nil, "TRANSPORT_MODE"},
		{req.UDPListenAddr != nil, "UDP_LISTEN_ADDR"},
		{req.NodeAddr != nil, "NODE_ADDR"},
		{req.MyCall != nil, "MY_CALL"},
		{req.MaxMessageLength != nil, "MAX_MESSAGE_LENGTH"},
		{req.LogLevel != nil, "LOG_LEVEL"},
	}
	if req.ReceiveLog != nil {
		rl := req.ReceiveLog
		checks = append(checks,
			check{rl.Enabled != nil, "RECEIVE_LOG_ENABLED"},
			check{rl.Path != nil, "RECEIVE_LOG_PATH"},
			check{rl.RetentionDays != nil, "RECEIVE_LOG_RETENTION_DAYS"},
			check{rl.ReplayWindow != nil, "RECEIVE_LOG_REPLAY_WINDOW"},
		)
	}
	if req.Stats != nil {
		s2 := req.Stats
		checks = append(checks,
			check{s2.Enabled != nil, "STATS_ENABLED"},
			check{s2.Path != nil, "STATS_PATH"},
			check{s2.RetentionDays != nil, "STATS_RETENTION_DAYS"},
		)
	}
	if req.ChatLog != nil {
		cl := req.ChatLog
		checks = append(checks,
			check{cl.Path != nil, "CHAT_LOG_PATH"},
			check{cl.HistoryWindow != nil, "CHAT_LOG_HISTORY_WINDOW"},
			check{cl.MaxHistoryWindow != nil, "CHAT_LOG_MAX_HISTORY_WINDOW"},
		)
	}
	if req.Send != nil {
		snd := req.Send
		checks = append(checks,
			check{snd.DedupTTL != nil, "SEND_DEDUP_TTL"},
		)
	}
	if req.Forward != nil && req.Forward.Targets != nil {
		checks = append(checks, check{true, "FORWARD_TARGETS"})
	}
	if req.Auth != nil {
		a := req.Auth
		checks = append(checks,
			check{a.Username != nil, "AUTH_USERNAME"},
			check{a.Password != nil && *a.Password != "" && *a.Password != passwordMask, "AUTH_PASSWORD"},
			check{a.SessionTTL != nil, "AUTH_SESSION_TTL"},
			check{a.CookieName != nil, "AUTH_COOKIE_NAME"},
		)
	}
	if req.RequestLog != nil && req.RequestLog.Enabled != nil {
		checks = append(checks, check{true, "REQUEST_LOG_ENABLED"})
	}
	if req.Storage != nil {
		st := req.Storage
		checks = append(checks,
			check{st.SQLitePath != nil, "STORAGE_SQLITE_PATH"},
			check{st.PurgeInterval != nil, "STORAGE_PURGE_INTERVAL"},
			check{st.ReceiveLogRetention != nil, "STORAGE_RECEIVE_LOG_RETENTION"},
			check{st.PublicChatRetention != nil, "STORAGE_PUBLIC_CHAT_RETENTION"},
			check{st.NodesRetention != nil, "STORAGE_NODES_RETENTION"},
			check{st.TelemetryRetention != nil, "STORAGE_TELEMETRY_RETENTION"},
		)
	}
	if req.Serial != nil {
		serial := req.Serial
		checks = append(checks,
			check{serial.Device != nil, "SERIAL_DEVICE"},
			check{serial.Baud != nil, "SERIAL_BAUD"},
			check{serial.DataBits != nil, "SERIAL_DATA_BITS"},
			check{serial.Parity != nil, "SERIAL_PARITY"},
			check{serial.StopBits != nil, "SERIAL_STOP_BITS"},
			check{serial.FlowControl != nil, "SERIAL_FLOW_CONTROL"},
			check{serial.DTR != nil, "SERIAL_DTR"},
			check{serial.RTS != nil, "SERIAL_RTS"},
			check{serial.ReadTimeout != nil, "SERIAL_READ_TIMEOUT"},
			check{serial.ReconnectInitial != nil, "SERIAL_RECONNECT_INITIAL"},
			check{serial.ReconnectMax != nil, "SERIAL_RECONNECT_MAX"},
			check{serial.StableResetAfter != nil, "SERIAL_STABLE_RESET_AFTER"},
			check{serial.MaxRecordBytes != nil, "SERIAL_MAX_RECORD_BYTES"},
		)
	}
	if req.NetConsole != nil {
		netconsole := req.NetConsole
		checks = append(checks,
			check{netconsole.Address != nil, "NETCONSOLE_ADDRESS"},
			check{netconsole.Password != nil && *netconsole.Password != passwordMask, "NETCONSOLE_PASSWORD"},
			check{netconsole.ConnectTimeout != nil, "NETCONSOLE_CONNECT_TIMEOUT"},
			check{netconsole.AuthTimeout != nil, "NETCONSOLE_AUTH_TIMEOUT"},
			check{netconsole.WriteTimeout != nil, "NETCONSOLE_WRITE_TIMEOUT"},
			check{netconsole.ReconnectInitial != nil, "NETCONSOLE_RECONNECT_INITIAL"},
			check{netconsole.ReconnectMax != nil, "NETCONSOLE_RECONNECT_MAX"},
			check{netconsole.StableResetAfter != nil, "NETCONSOLE_STABLE_RESET_AFTER"},
			check{netconsole.MaxAuthLineBytes != nil, "NETCONSOLE_MAX_AUTH_LINE_BYTES"},
			check{netconsole.MaxRecordBytes != nil, "NETCONSOLE_MAX_RECORD_BYTES"},
		)
	}

	for _, c := range checks {
		if c.present && env[c.suffix] {
			return fmt.Errorf("field %s is managed by environment variable GOMESHCOM_%s and cannot be changed via API", c.suffix, c.suffix)
		}
	}
	return nil
}
