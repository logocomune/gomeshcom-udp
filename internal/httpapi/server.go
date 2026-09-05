package httpapi

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/logocomune/gomeshcom-client/internal/channelshow"
	"github.com/logocomune/gomeshcom-client/internal/chatlog"
	"github.com/logocomune/gomeshcom-client/internal/chatstatus"
	"github.com/logocomune/gomeshcom-client/internal/config"
	"github.com/logocomune/gomeshcom-client/internal/dmstats"
	"github.com/logocomune/gomeshcom-client/internal/events"
	"github.com/logocomune/gomeshcom-client/internal/meshcom"
	"github.com/logocomune/gomeshcom-client/internal/outbox"
	"github.com/logocomune/gomeshcom-client/internal/positions"
	"github.com/logocomune/gomeshcom-client/internal/receivelog"
	"github.com/logocomune/gomeshcom-client/internal/sendcache"
	"github.com/logocomune/gomeshcom-client/internal/station"
	"github.com/logocomune/gomeshcom-client/internal/stats"
	"github.com/logocomune/gomeshcom-client/internal/transport"
	"github.com/logocomune/gomeshcom-client/internal/udpbridge"
	"github.com/logocomune/gomeshcom-client/internal/webui"
)

type stationIdentityEvent struct {
	Callsign           string `json:"callsign"`
	Version            string `json:"version,omitempty"`
	TxDisabled         bool   `json:"txDisabled"`
	ForwardTargetCount int    `json:"forwardTargetCount"`
}

// messageSender abstracts udpbridge.Bridge.SendText for testing.
type messageSender interface {
	SendText(ctx context.Context, destination, message string, maxLength int) error
}

type Server struct {
	cfg          config.Config
	identity     *station.Identity
	version      string
	bus          *events.Bus
	positions    *positions.Store
	receiveLog   *receivelog.Logger
	chatLog      *chatlog.Logger
	chatStatus   *chatstatus.Store
	channelShow  *channelshow.Store
	statsStore   *stats.Store
	dmStats      *dmstats.Store
	bridge       messageSender
	sendCache    *sendcache.Cache
	outbox       *outbox.Outbox
	sessions     *sessionStore
	sessionDB    *sql.DB
	cancel       context.CancelFunc
	envOverrides config.EnvOverrides
	tomlPath     string
	restartFunc  func()
	shutdownFunc func()
	startedAt    time.Time
	transport    transport.StatusProvider
}

const outgoingEchoTimeout = 5 * time.Second

type ServerOption func(*Server)

func WithChannelShow(store *channelshow.Store) ServerOption {
	return func(server *Server) {
		server.channelShow = store
	}
}

// WithStationIdentity attaches a runtime station identity to the server.
// When provided, the server uses the live callsign from identity instead of
// the startup config value.
func WithStationIdentity(id *station.Identity) ServerOption {
	return func(server *Server) {
		server.identity = id
	}
}

// WithStats attaches a stats store to the server, enabling the /api/stats endpoint.
func WithStats(store *stats.Store) ServerOption {
	return func(server *Server) {
		server.statsStore = store
	}
}

// WithDMStats attaches a DM stats store to the server, enabling the
// /api/stats/dm endpoint and per-callsign sent/ack tracking.
func WithDMStats(store *dmstats.Store) ServerOption {
	return func(server *Server) {
		server.dmStats = store
	}
}

func WithSessionDB(db *sql.DB) ServerOption {
	return func(server *Server) {
		server.sessionDB = db
	}
}

// WithEnvOverrides records which config fields are managed by environment variables.
// The server uses this to mark fields read-only in GET /api/config responses and
// to reject updates for those fields via PUT /api/config.
func WithEnvOverrides(env config.EnvOverrides) ServerOption {
	return func(server *Server) {
		server.envOverrides = env
	}
}

// WithTomlPath sets the path to the TOML config file.
// PUT /api/config writes updated config to this path atomically.
func WithTomlPath(path string) ServerOption {
	return func(server *Server) {
		server.tomlPath = path
	}
}

// WithRestartFunc registers a callback that POST /api/restart (and SIGHUP) will
// invoke to initiate a graceful restart. If not set, the endpoint returns 501.
func WithRestartFunc(fn func()) ServerOption {
	return func(server *Server) {
		server.restartFunc = fn
	}
}

// WithShutdownFunc registers a callback that POST /api/shutdown will invoke
// to initiate a graceful shutdown without restarting. If not set, the endpoint
// returns 501.
func WithShutdownFunc(fn func()) ServerOption {
	return func(server *Server) {
		server.shutdownFunc = fn
	}
}

func WithTransportStatus(provider transport.StatusProvider) ServerOption {
	return func(server *Server) {
		server.transport = provider
	}
}

func NewServer(cfg config.Config, version string, bus *events.Bus, positionStore *positions.Store, receiveLog *receivelog.Logger, chatLog *chatlog.Logger, bridge messageSender, sc *sendcache.Cache, chatStatus *chatstatus.Store, options ...ServerOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		cfg:        cfg,
		identity:   station.NewInMemory(cfg.MyCall), // overridable via WithStationIdentity
		version:    version,
		bus:        bus,
		positions:  positionStore,
		receiveLog: receiveLog,
		chatLog:    chatLog,
		chatStatus: chatStatus,
		bridge:     bridge,
		sendCache:  sc,
		cancel:     cancel,
		startedAt:  time.Now().UTC(),
	}
	for _, option := range options {
		option(server)
	}
	server.outbox = outbox.New(outgoingEchoTimeout, server.handleOutgoingTimeout)
	server.watchOutgoingEchoes(ctx)
	if authEnabled(cfg) {
		if server.sessionDB != nil {
			server.sessions = newSQLiteSessionStore(server.sessionDB)
		} else {
			server.sessions = newMemorySessionStore()
		}
		server.sessions.start(ctx)
	}
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/session", s.sessionStatus)
	mux.HandleFunc("POST /api/session", s.createSession)
	mux.HandleFunc("DELETE /api/session", s.deleteSession)
	mux.Handle("GET /api/positions", requireAuth(http.HandlerFunc(s.listPositions), s))
	mux.Handle("POST /api/messages", requireAuth(http.HandlerFunc(s.createMessage), s))
	mux.Handle("GET /api/events", requireAuth(http.HandlerFunc(s.streamEvents), s))
	mux.Handle("GET /api/channel-show", requireAuth(http.HandlerFunc(s.getChannelShow), s))
	mux.Handle("PUT /api/channel-show", requireAuth(http.HandlerFunc(s.updateChannelShow), s))
	mux.Handle("GET /api/adm/configs/my-call", requireAuth(http.HandlerFunc(s.getMyCall), s))
	mux.Handle("PUT /api/adm/configs/my-call", requireAuth(http.HandlerFunc(s.updateMyCall), s))
	mux.Handle("GET /api/config", requireAuth(http.HandlerFunc(s.getConfig), s))
	mux.Handle("PUT /api/config", requireAuth(http.HandlerFunc(s.updateConfig), s))
	mux.Handle("POST /api/restart", requireAuth(http.HandlerFunc(s.restart), s))
	mux.Handle("POST /api/shutdown", requireAuth(http.HandlerFunc(s.shutdown), s))
	mux.Handle("GET /api/chat/list", requireAuth(http.HandlerFunc(s.listConversations), s))
	mux.Handle("GET /api/chat/{conversation}", requireAuth(http.HandlerFunc(s.getConversation), s))
	mux.Handle("DELETE /api/chat/{conversation}", requireAuth(http.HandlerFunc(s.deleteConversation), s))
	mux.Handle("POST /api/chat/{conversation}/read", requireAuth(http.HandlerFunc(s.markConversationRead), s))
	mux.Handle("GET /api/stats", requireAuth(http.HandlerFunc(s.listStats), s))
	mux.Handle("GET /api/stats/dm", requireAuth(http.HandlerFunc(s.listDMStats), s))
	mux.Handle("/", spaHandler(webui.FS()))
	handler := cacheHeadersMiddleware(mux)
	if s.cfg.Compression.Enabled {
		handler = compressionMiddleware(compressionOptions{
			MinimumSize: s.cfg.Compression.MinimumSize,
			GzipLevel:   gzip.BestSpeed,
		}, handler)
	}
	if s.cfg.RequestLog.Enabled {
		return requestLogMiddleware(handler)
	}
	return handler
}

func cacheHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCacheHeaders(w.Header(), r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func setCacheHeaders(header http.Header, path string) {
	switch {
	case strings.HasPrefix(path, "/api/"):
		setNoCacheHeaders(header)
	case strings.HasPrefix(path, "/_app/immutable/"):
		setImmutableCacheHeaders(header)
	case path == "/" || path == "/index.html" || path == "/service-worker.js" || path == "/manifest.webmanifest":
		setIndexNoCacheHeaders(header)
	}
}

func setNoCacheHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

func setImmutableCacheHeaders(header http.Header) {
	header.Set("Cache-Control", "public, max-age=31536000, immutable")
}

func setIndexNoCacheHeaders(header http.Header) {
	header.Set("Cache-Control", "no-cache, must-revalidate")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

type flushStatusRecorder struct {
	*statusRecorder
}

func (r *flushStatusRecorder) Flush() {
	r.ResponseWriter.(http.Flusher).Flush()
}

func requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now().UTC()
		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		responseWriter := http.ResponseWriter(recorder)
		if _, ok := w.(http.Flusher); ok {
			responseWriter = &flushStatusRecorder{statusRecorder: recorder}
		}

		next.ServeHTTP(responseWriter, r)

		duration := time.Since(startedAt)
		slog.Info("http request",
			"method", r.Method,
			"endpoint", r.URL.Path,
			"status", recorder.statusCode,
			"caller_ip", callerIP(r),
			"started_at", startedAt.Format(time.RFC3339Nano),
			"duration", duration.String(),
			"duration_ms", float64(duration.Microseconds())/1000,
		)
	})
}

func callerIP(r *http.Request) string {
	for _, header := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value != "" {
			return value
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(fsys, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path — serve SPA entry point so client-side routing works.
		setIndexNoCacheHeaders(w.Header())
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	response := map[string]any{
		"status":     "ok",
		"version":    s.version,
		"callsign":   s.identity.Current(),
		"started_at": s.startedAt.Format(time.RFC3339Nano),
	}
	if s.transport != nil {
		status := s.transport.TransportStatus()
		response["transport"] = status
		switch status.State {
		case transport.StateConnected:
		default:
			response["status"] = "degraded"
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) listPositions(w http.ResponseWriter, _ *http.Request) {
	if s.positions == nil {
		writeJSON(w, http.StatusOK, map[string]positions.Record{})
		return
	}

	writeJSON(w, http.StatusOK, s.positions.Snapshot())
}

const (
	defaultStatsHours = 24
	maxStatsHours     = 720
)

func (s *Server) listStats(w http.ResponseWriter, r *http.Request) {
	if s.statsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"buckets": []stats.Bucket{}})
		return
	}

	hours := defaultStatsHours
	if h := r.URL.Query().Get("hours"); h != "" {
		parsed, err := strconv.Atoi(h)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "hours must be a positive integer")
			return
		}
		if parsed > maxStatsHours {
			parsed = maxStatsHours
		}
		hours = parsed
	}

	to := time.Now().UTC()
	from := to.Add(-time.Duration(hours) * time.Hour)

	buckets, err := s.statsStore.ReadRange(from, to)
	if err != nil {
		slog.Error("stats: read range failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"from":    from.Format(time.RFC3339),
		"to":      to.Format(time.RFC3339),
		"hours":   hours,
		"buckets": buckets,
	})
}

// dmStatsEntry is the JSON response shape for a single callsign in /api/stats/dm.
type dmStatsEntry struct {
	Sent int `json:"sent"`
	Ack  int `json:"ack"`
}

func (s *Server) listDMStats(w http.ResponseWriter, _ *http.Request) {
	if s.dmStats == nil {
		writeJSON(w, http.StatusOK, map[string]dmStatsEntry{})
		return
	}

	snapshot := s.dmStats.Snapshot()
	result := make(map[string]dmStatsEntry, len(snapshot))
	for callsign, e := range snapshot {
		result[callsign] = dmStatsEntry{
			Sent: e.Sent,
			Ack:  e.Ack,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createMessage(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Destination string `json:"dst"`
		Message     string `json:"msg"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<13) // 8 KB
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	outgoing, err := meshcom.NewOutgoingText(request.Destination, request.Message, s.cfg.MaxMessageLength)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.sendCache != nil && !s.sendCache.Reserve(outgoing.Destination, outgoing.Message) {
		w.Header().Set("Retry-After", "2")
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":          "duplicate",
			"retry_after_ms": 2000,
		})
		return
	}

	if s.bridge == nil {
		writeError(w, http.StatusServiceUnavailable, "bridge not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var pending outbox.PendingMessage
	if s.outbox != nil && !s.cfg.DemoMode {
		pending = s.outbox.Register(s.identity.Current(), outgoing.Destination, outgoing.Message, time.Now().UTC())
	}
	if err := s.bridge.SendText(ctx, outgoing.Destination, outgoing.Message, s.cfg.MaxMessageLength); err != nil {
		s.outbox.Cancel(pending.ID)
		slog.Error("message send failed", "transport", s.cfg.TransportMode, "error", err)
		if errors.Is(err, transport.ErrUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "transport unavailable")
			return
		}
		if errors.Is(err, udpbridge.ErrNodeNotDetected) {
			writeError(w, http.StatusServiceUnavailable, "node not yet detected")
			return
		}
		writeError(w, http.StatusBadGateway, "send failed")
		return
	}
	slog.Info("message sent", "dst", outgoing.Destination, "msg", outgoing.Message)
	if s.dmStats != nil {
		s.dmStats.RecordSent(outgoing.Destination)
	}

	writeJSON(w, http.StatusAccepted, outgoing)
}

func (s *Server) watchOutgoingEchoes(ctx context.Context) {
	if s.bus == nil {
		return
	}
	subscriber := s.bus.Subscribe(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-subscriber:
				if !ok {
					return
				}
				if event.Type != "packet.received" {
					continue
				}
				message, ok := textMessageFromEvent(event)
				if !ok {
					continue
				}
				if s.outbox != nil {
					if _, ok := s.outbox.Confirm(message.Source, message.Destination, message.Message); ok {
						now := time.Now().UTC()
						if s.dmStats != nil {
							s.dmStats.RecordAck(message.Destination)
						}
						if s.bus != nil {
							s.bus.Publish(events.Event{
								Type: "message.delivered",
								Data: chatlog.Record{
									ReceivedAt:     now,
									Src:            message.Source,
									Dst:            message.Destination,
									Msg:            message.Message,
									Direction:      "outbound",
									DeliveryStatus: "delivered",
								},
							})
						}
					}
				}
			}
		}
	}()
}

func textMessageFromEvent(event events.Event) (meshcom.TextMessage, bool) {
	data, ok := event.Data.(map[string]any)
	if !ok {
		return meshcom.TextMessage{}, false
	}
	message, ok := data["packet"].(meshcom.TextMessage)
	return message, ok
}

func (s *Server) handleOutgoingTimeout(message outbox.PendingMessage) {
	record := chatlog.Record{
		ReceivedAt:     time.Now().UTC(),
		Src:            message.Source,
		Dst:            message.Destination,
		Msg:            message.Message,
		Direction:      "outbound",
		DeliveryStatus: "failed",
	}
	if s.chatLog != nil {
		var err error
		record, err = s.chatLog.AppendFailed(message.Source, message.Destination, message.Message, record.ReceivedAt)
		if err != nil {
			slog.Error("failed outgoing chat log write failed", "error", err)
			return
		}
	}
	if s.bus != nil {
		s.bus.Publish(events.Event{Type: "message.failed", Data: record})
	}
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	replayCutoff, replayEnabled, err := s.replayCutoff(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	setNoCacheHeaders(w.Header())
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	subscriber := s.bus.Subscribe(r.Context())
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	if err := writeHeartbeat(w); err != nil {
		return
	}
	flusher.Flush()

	forwardTargets, _ := config.ParseForwardTargets(s.cfg.Forward.Targets)
	if err := writeSSE(w, events.Event{Type: "station.identity", Data: stationIdentityEvent{
		Callsign:           s.identity.Current(),
		Version:            s.version,
		TxDisabled:         s.cfg.DemoMode,
		ForwardTargetCount: len(forwardTargets),
	}}); err != nil {
		return
	}
	flusher.Flush()

	if s.positions != nil {
		if err := writeSSE(w, events.Event{Type: "positions.snapshot", Data: s.positions.Snapshot()}); err != nil {
			return
		}
		flusher.Flush()
	}

	if s.chatStatus != nil {
		if err := writeSSE(w, events.Event{Type: "chatstatus.snapshot", Data: s.chatStatus.Snapshot()}); err != nil {
			return
		}
		flusher.Flush()
	}

	if err := writeSSE(w, events.Event{Type: "channelshow.snapshot", Data: s.channelShowSnapshot()}); err != nil {
		return
	}
	flusher.Flush()

	if err := s.replayRecentPackets(w, flusher, replayCutoff, replayEnabled); err != nil {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-subscriber:
			if !ok {
				return
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if err := writeHeartbeat(w); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// scopeFromRequest returns "basecall" when ?scope=basecall is set, else "mycall".
func scopeFromRequest(r *http.Request) string {
	if r.URL.Query().Get("scope") == "basecall" {
		return "basecall"
	}
	return "mycall"
}

// dmSSIDFromID extracts the caller segment (first segment after "DM_") from a
// DM conversation id of the form DM_<caller>_<peer>. Returns "" for non-DM ids
// or legacy DM_<peer> ids without a separator.
func dmSSIDFromID(id string) string {
	if !strings.HasPrefix(id, "DM_") {
		return ""
	}
	rest := id[3:]
	idx := strings.Index(rest, "_")
	if idx < 0 {
		return "" // legacy: no caller segment
	}
	return rest[:idx]
}

// isDMKeyForBasecallAndPeer reports whether a chatstatus key belongs to the
// given operator basecall and peer. Used for basecall-scope aggregation.
func isDMKeyForBasecallAndPeer(key, myBase, peer string) bool {
	if !strings.HasPrefix(key, "DM_") {
		return false
	}
	if chatlog.DMPeer(key) != peer {
		return false
	}
	caller := dmSSIDFromID(key)
	if caller == "" {
		return false // legacy key: skip
	}
	return chatlog.BaseCall(caller) == myBase
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	if s.chatLog == nil {
		writeJSON(w, http.StatusOK, []chatlog.Conversation{})
		return
	}
	convs, err := s.chatLog.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list conversations: "+err.Error())
		return
	}

	scope := scopeFromRequest(r)
	myCall := s.identity.Current()
	myBase := chatlog.BaseCall(myCall)

	result := make([]chatlog.Conversation, 0, len(convs))
	for _, conv := range convs {
		if !strings.HasPrefix(conv.ID, "DM_") {
			// P_* conversations: always include unchanged.
			result = append(result, conv)
			continue
		}
		// Extract the basecall segment from the file id DM_<basecall>_<peer>.
		fileBase := dmSSIDFromID(conv.ID) // for file ids, "caller" segment = basecall
		if fileBase == "" {
			// Legacy DM_<peer> file; include as-is (migration may not have run yet).
			result = append(result, conv)
			continue
		}
		if fileBase != myBase {
			// Belongs to a different operator.
			continue
		}
		peer := chatlog.DMPeer(conv.ID)
		if scope == "basecall" {
			// Return the file id unchanged; it already uses the basecall form.
			result = append(result, conv)
		} else {
			// mycall scope: include only if the shared file has at least one
			// record where the active full SSID appears as Src or Dst.
			has, err := s.chatLog.FileContainsSSID(conv.ID, myCall)
			if err != nil {
				slog.Warn("chat list: ssid probe failed", "file", conv.ID, "error", err)
			}
			if !has {
				continue
			}
			conv.ID = "DM_" + chatlog.Sanitize(myCall) + "_" + peer
			result = append(result, conv)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("conversation")
	if !chatlog.ValidConversationID(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	// Resolve the API id to the .jsonl file id (basecall form).
	fileID := chatlog.FileIDForAPIID(id)
	if !chatlog.ValidConversationID(fileID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	defaultHours := s.defaultChatHistoryHours(id)
	maxHours := s.cfg.ChatLog.MaxHistoryWindow.Hours()

	hours := defaultHours
	if raw := r.URL.Query().Get("hours"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "invalid hours parameter")
			return
		}
		hours = parsed
	}
	if hours > maxHours {
		hours = maxHours
	}

	since := time.Now().UTC().Add(-time.Duration(math.Round(hours * float64(time.Hour))))

	if s.chatLog == nil {
		writeJSON(w, http.StatusOK, []chatlog.Record{})
		return
	}

	records, err := s.chatLog.ReadSince(fileID, since)
	if err != nil {
		if errors.Is(err, chatlog.ErrInvalidID) {
			writeError(w, http.StatusBadRequest, "invalid conversation id")
			return
		}
		writeError(w, http.StatusInternalServerError, "read conversation: "+err.Error())
		return
	}

	// In mycall scope, filter records to those whose my-side callsign matches
	// the SSID embedded in the API id (DM_<ssid>_<peer>).
	if scopeFromRequest(r) == "mycall" && strings.HasPrefix(id, "DM_") {
		ssid := dmSSIDFromID(id)
		if ssid != "" {
			filtered := records[:0]
			for _, rec := range records {
				if chatlog.RecordMatchesSSID(rec.Src, rec.Dst, ssid) {
					filtered = append(filtered, rec)
				}
			}
			records = filtered
		}
	}

	writeJSON(w, http.StatusOK, records)
}

func (s *Server) defaultChatHistoryHours(conversationID string) float64 {
	if strings.HasPrefix(conversationID, "DM_") {
		return (30 * 24 * time.Hour).Hours()
	}
	return s.cfg.ChatLog.HistoryWindow.Hours()
}

func (s *Server) markConversationRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("conversation")
	if !chatlog.ValidConversationID(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if s.chatStatus != nil {
		now := time.Now().UTC()
		if scopeFromRequest(r) == "basecall" && strings.HasPrefix(id, "DM_") {
			// Mark all per-SSID status keys for this operator + peer as read.
			myBase := chatlog.BaseCall(s.identity.Current())
			peer := chatlog.DMPeer(id)
			for key := range s.chatStatus.Snapshot() {
				if isDMKeyForBasecallAndPeer(key, myBase, peer) {
					s.chatStatus.MarkRead(key, now)
				}
			}
			if err := s.chatStatus.SaveIfDirty(); err != nil {
				slog.Error("chat status save after basecall mark-read failed", "id", id, "error", err)
			}
		} else {
			s.chatStatus.MarkRead(id, now)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("conversation")
	if !chatlog.ValidConversationID(id) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if s.chatLog == nil {
		writeError(w, http.StatusServiceUnavailable, "chatlog disabled")
		return
	}
	// Remove the .jsonl file using the basecall file id.
	fileID := chatlog.FileIDForAPIID(id)
	if err := s.chatLog.Remove(fileID); err != nil {
		writeError(w, http.StatusInternalServerError, "remove conversation: "+err.Error())
		return
	}
	if s.chatStatus != nil {
		if strings.HasPrefix(id, "DM_") {
			// Remove all per-SSID status keys that belong to this operator + peer
			// so no orphan entries remain regardless of which scope was used.
			myBase := chatlog.BaseCall(s.identity.Current())
			peer := chatlog.DMPeer(id)
			for key := range s.chatStatus.Snapshot() {
				if isDMKeyForBasecallAndPeer(key, myBase, peer) {
					s.chatStatus.Remove(key)
				}
			}
		} else {
			s.chatStatus.Remove(id)
		}
		if err := s.chatStatus.SaveIfDirty(); err != nil {
			slog.Error("chat status save after conversation delete failed", "id", id, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) replayCutoff(r *http.Request) (time.Time, bool, error) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	var cutoff time.Time
	var hasCutoff bool
	if from != "" {
		parsed, err := time.Parse(time.RFC3339Nano, from)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("invalid from timestamp")
		}
		cutoff = parsed.UTC()
		hasCutoff = true
	}

	if s.cfg.ReceiveLog.ReplayWindow > 0 {
		minCutoff := time.Now().UTC().Add(-s.cfg.ReceiveLog.ReplayWindow)
		if !hasCutoff || cutoff.Before(minCutoff) {
			cutoff = minCutoff
		}
		return cutoff, true, nil
	}

	if hasCutoff {
		return cutoff, true, nil
	}
	return time.Time{}, false, nil
}

func writeHeartbeat(w http.ResponseWriter) error {
	_, err := fmt.Fprint(w, "event: heartbeat\ndata: {}\n\n")
	return err
}

func (s *Server) replayRecentPackets(w http.ResponseWriter, flusher http.Flusher, cutoff time.Time, enabled bool) error {
	if s.receiveLog == nil || !enabled {
		return nil
	}

	records, err := s.receiveLog.ReadSince(cutoff)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.ParseError != "" || record.Raw == "" {
			continue
		}

		packet, err := meshcom.ParsePacket([]byte(record.Raw))
		if err != nil {
			continue
		}
		if err := writeSSE(w, events.Event{
			Type: "packet.received",
			Data: map[string]any{
				"remote_addr": record.RemoteAddr,
				"packet":      packet.Packet,
				"received_at": record.ReceivedAt.UTC().Format(time.RFC3339Nano),
				"replay":      true,
			},
		}); err != nil {
			return err
		}
		flusher.Flush()
	}

	return nil
}

func writeSSE(w http.ResponseWriter, event events.Event) error {
	payload, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("json encode failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}
