package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/logocomune/gomeshcom-client/internal/apprestart"
	"github.com/logocomune/gomeshcom-client/internal/channelshow"
	"github.com/logocomune/gomeshcom-client/internal/chatlog"
	"github.com/logocomune/gomeshcom-client/internal/chatstatus"
	"github.com/logocomune/gomeshcom-client/internal/config"
	"github.com/logocomune/gomeshcom-client/internal/dmstats"
	"github.com/logocomune/gomeshcom-client/internal/events"
	"github.com/logocomune/gomeshcom-client/internal/httpapi"
	"github.com/logocomune/gomeshcom-client/internal/legacymigrate"
	"github.com/logocomune/gomeshcom-client/internal/logfmt"
	"github.com/logocomune/gomeshcom-client/internal/netconsole"
	"github.com/logocomune/gomeshcom-client/internal/packetingest"
	"github.com/logocomune/gomeshcom-client/internal/positions"
	"github.com/logocomune/gomeshcom-client/internal/receivelog"
	"github.com/logocomune/gomeshcom-client/internal/sendcache"
	"github.com/logocomune/gomeshcom-client/internal/serialbridge"
	"github.com/logocomune/gomeshcom-client/internal/station"
	"github.com/logocomune/gomeshcom-client/internal/stats"
	"github.com/logocomune/gomeshcom-client/internal/storage"
	"github.com/logocomune/gomeshcom-client/internal/telemetry"
	"github.com/logocomune/gomeshcom-client/internal/transport"
	"github.com/logocomune/gomeshcom-client/internal/udpbridge"
	"github.com/logocomune/gomeshcom-client/internal/udpforward"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const startupBannerInnerWidth = 60

type nodeTransport interface {
	SendText(ctx context.Context, destination, message string, maxLength int) error
	transport.StatusProvider
}

func main() {
	restart, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if restart {
		if apprestart.RunningInContainer() {
			// Inside a container: exit cleanly and let the container runtime
			// relaunch via its restart policy (e.g. docker-compose restart:
			// unless-stopped).
			os.Exit(0)
		}

		// Standalone: give the OS a moment to release the TCP/UDP ports and
		// file locks before the child tries to bind them.
		time.Sleep(500 * time.Millisecond)
		if err := apprestart.RestartSelf(); err != nil {
			fmt.Fprintln(os.Stderr, "restart failed:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func run() (bool, error) {
	cfg, envOverrides, info, err := config.Load(version)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) || errors.Is(err, conf.ErrVersionWanted) {
			fmt.Println(info)
			return false, nil
		}
		return false, fmt.Errorf("load config: %w", err)
	}
	configureLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// restartRequested is set to true when a graceful restart (not a clean
	// shutdown) was requested via POST /api/restart or SIGHUP.
	var restartRequested atomic.Bool

	// triggerRestart marks a restart intent and cancels the main context so
	// that the graceful HTTP+UDP shutdown sequence runs before main() decides
	// whether to exit or re-exec.
	triggerRestart := func() {
		restartRequested.Store(true)
		stop()
	}

	// Watch for SIGHUP as an additional restart trigger (useful for ops/scripts).
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		select {
		case <-hup:
			slog.Info("SIGHUP received, restarting")
			triggerRestart()
		case <-ctx.Done():
		}
	}()

	if err := ensureDataDirs(cfg.DataDir); err != nil {
		return false, err
	}
	if err := runLegacyDataMigration(cfg); err != nil {
		return false, err
	}

	sqliteDB, err := openStorageAndImport(ctx, cfg)
	if err != nil {
		return false, err
	}
	defer sqliteDB.Close()
	go sqliteDB.StartPurge(ctx, storage.PurgePolicy{
		Interval:            cfg.Storage.PurgeInterval,
		ReceiveLogRetention: cfg.Storage.ReceiveLogRetention,
		PublicChatRetention: cfg.Storage.PublicChatRetention,
		NodesRetention:      cfg.Storage.NodesRetention,
		TelemetryRetention:  cfg.Storage.TelemetryRetention,
	})

	// Runtime station identity: persisted value wins over GOMESHCOM_MY_CALL default.
	stationIdentity, err := station.NewSQLite(sqliteDB.SQL(), cfg.MyCall)
	if err != nil {
		return false, fmt.Errorf("load station identity: %w", err)
	}
	go stationIdentity.Start(ctx)

	bus := events.NewBus()
	positionStore := positions.NewSQLite(sqliteDB.SQL())
	if err := positionStore.Load(); err != nil {
		return false, fmt.Errorf("load positions: %w", err)
	}
	go positionStore.Start(ctx)

	receiveLogger := receivelog.NewSQLite(receivelog.Config{
		Enabled:       cfg.ReceiveLog.Enabled,
		Path:          cfg.ReceiveLog.Path,
		RetentionDays: cfg.ReceiveLog.RetentionDays,
	}, sqliteDB.SQL())
	chatLogger := chatlog.NewSQLite(sqliteDB.SQL(), stationIdentity)
	chatStatus, err := chatstatus.NewSQLite(sqliteDB.SQL())
	if err != nil {
		return false, fmt.Errorf("load chat status: %w", err)
	}
	go chatStatus.Start(ctx)

	channelShow, err := channelshow.NewSQLite(sqliteDB.SQL())
	if err != nil {
		return false, fmt.Errorf("load channel show: %w", err)
	}
	go channelShow.Start(ctx)

	var statsStore *stats.Store
	if cfg.Stats.Enabled {
		statsStore = stats.NewSQLite(sqliteDB.SQL(), stats.Config{
			Enabled:       true,
			Path:          cfg.Stats.Path,
			RetentionDays: cfg.Stats.RetentionDays,
		})
		if err := statsStore.Load(); err != nil {
			return false, fmt.Errorf("load stats: %w", err)
		}
		go statsStore.Start(ctx)
		collector := stats.NewCollector(statsStore, positionStore, stationIdentity)
		go collector.Run(ctx, bus)
	}

	dmStatsStore := dmstats.NewSQLite(sqliteDB.SQL())
	if err := dmStatsStore.Load(); err != nil {
		return false, fmt.Errorf("load dm stats: %w", err)
	}
	go dmStatsStore.Start(ctx)

	var fwd *udpforward.Forwarder
	if cfg.Forward.Targets != "" {
		targets, err := config.ParseForwardTargets(cfg.Forward.Targets)
		if err != nil {
			return false, fmt.Errorf("udp forward targets: %w", err)
		}
		fwd, err = udpforward.New(targets)
		if err != nil {
			return false, fmt.Errorf("udp forwarder: %w", err)
		}
		defer fwd.Close()
	}

	packetProcessor := packetingest.NewProcessor(packetingest.Dependencies{
		Bus:        bus,
		ReceiveLog: receiveLogger,
		ChatLog:    chatLogger,
		ChatStatus: chatStatus,
		Identity:   stationIdentity,
		Positions:  positionStore,
		Telemetry:  telemetry.NewSQLite(sqliteDB.SQL()),
	})
	var link nodeTransport
	var runTransport func(context.Context) error
	switch cfg.TransportMode {
	case config.TransportUDP:
		udpLink := udpbridge.NewBridgeWithProcessor(
			cfg.UDPListenAddr,
			cfg.NodeAddr,
			packetProcessor,
			cfg.DemoMode,
			fwd,
		)
		link = udpLink
		runTransport = udpLink.Listen
	case config.TransportSerial:
		serialLink, err := serialbridge.NewBridge(serialbridge.Options{
			Config: serialbridge.Config{
				Port: serialbridge.PortConfig{
					Device:      cfg.Serial.Device,
					Baud:        cfg.Serial.Baud,
					DataBits:    cfg.Serial.DataBits,
					Parity:      cfg.Serial.Parity,
					StopBits:    cfg.Serial.StopBits,
					DTR:         cfg.Serial.DTR,
					RTS:         cfg.Serial.RTS,
					ReadTimeout: cfg.Serial.ReadTimeout,
				},
				ReconnectInitial: cfg.Serial.ReconnectInitial,
				ReconnectMax:     cfg.Serial.ReconnectMax,
				StableResetAfter: cfg.Serial.StableResetAfter,
				MaxRecordBytes:   cfg.Serial.MaxRecordBytes,
			},
			Processor: packetProcessor,
			Forwarder: fwd,
			Identity:  stationIdentity,
			DisableTX: cfg.DemoMode,
		})
		if err != nil {
			return false, fmt.Errorf("configure serial transport: %w", err)
		}
		link = serialLink
		runTransport = serialLink.Run
	case config.TransportNetConsole:
		netConsoleLink, err := netconsole.NewBridge(netconsole.Options{
			Config: netconsole.Config{
				Address:          cfg.NetConsole.Address,
				Password:         cfg.NetConsole.Password,
				ConnectTimeout:   cfg.NetConsole.ConnectTimeout,
				AuthTimeout:      cfg.NetConsole.AuthTimeout,
				WriteTimeout:     cfg.NetConsole.WriteTimeout,
				ReconnectInitial: cfg.NetConsole.ReconnectInitial,
				ReconnectMax:     cfg.NetConsole.ReconnectMax,
				StableResetAfter: cfg.NetConsole.StableResetAfter,
				MaxAuthLineBytes: cfg.NetConsole.MaxAuthLineBytes,
				MaxRecordBytes:   cfg.NetConsole.MaxRecordBytes,
			},
			Processor: packetProcessor,
			Forwarder: fwd,
			Identity:  stationIdentity,
			DisableTX: cfg.DemoMode,
		})
		if err != nil {
			return false, fmt.Errorf("configure netconsole transport: %w", err)
		}
		link = netConsoleLink
		runTransport = netConsoleLink.Run
	default:
		return false, fmt.Errorf("unsupported transport mode %q", cfg.TransportMode)
	}
	transportDone := make(chan error, 1)
	go func() {
		transportDone <- runTransport(ctx)
	}()

	sc := sendcache.New(cfg.Send.DedupTTL)
	serverOpts := []httpapi.ServerOption{
		httpapi.WithChannelShow(channelShow),
		httpapi.WithStationIdentity(stationIdentity),
		httpapi.WithSessionDB(sqliteDB.SQL()),
	}
	if statsStore != nil {
		serverOpts = append(serverOpts, httpapi.WithStats(statsStore))
	}
	serverOpts = append(serverOpts, httpapi.WithDMStats(dmStatsStore))
	serverOpts = append(serverOpts,
		httpapi.WithEnvOverrides(envOverrides),
		httpapi.WithTomlPath(config.DefaultTomlPath(cfg.DataDir)),
		httpapi.WithRestartFunc(triggerRestart),
		httpapi.WithShutdownFunc(stop),
		httpapi.WithTransportStatus(link),
	)
	apiServer := httpapi.NewServer(cfg, version, bus, positionStore, receiveLogger, chatLogger, link, sc, chatStatus, serverOpts...)
	defer apiServer.Close()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("http shutdown failed", "error", err)
		}
	}()

	printStartupBanner(cfg, stationIdentity.Current())

	slog.Info(
		"gomeshcom listening",
		"http_addr", cfg.HTTPAddr,
		"transport", cfg.TransportMode,
		"endpoint", link.TransportStatus().Endpoint,
	)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.ListenAndServe()
	}()

	var serverErr error
	var transportErr error
	serverFinished := false
	transportFinished := false
	transportStoppedUnexpectedly := false
	select {
	case serverErr = <-serverDone:
		serverFinished = true
		stop()
	case transportErr = <-transportDone:
		transportFinished = true
		transportStoppedUnexpectedly = ctx.Err() == nil
		stop()
	case <-ctx.Done():
	}

	if !serverFinished {
		select {
		case serverErr = <-serverDone:
			serverFinished = true
		case <-time.After(10 * time.Second):
			return false, errors.New("HTTP server did not stop within timeout")
		}
	}
	if !transportFinished {
		select {
		case transportErr = <-transportDone:
			transportFinished = true
		case <-time.After(10 * time.Second):
			return false, errors.New("node transport did not stop within timeout")
		}
	}
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return false, fmt.Errorf("serve http: %w", serverErr)
	}
	if transportStoppedUnexpectedly {
		if transportErr == nil {
			return false, fmt.Errorf("%s transport stopped unexpectedly", cfg.TransportMode)
		}
		return false, fmt.Errorf("%s transport stopped: %w", cfg.TransportMode, transportErr)
	}

	return restartRequested.Load(), nil
}

func runLegacyDataMigration(cfg config.Config) error {
	// DEPRECATED: one-time legacy data migration; remove after migration window closes.
	legacyCallsign, err := station.LoadLegacy(station.DefaultPath(cfg.DataDir), cfg.MyCall)
	if err != nil {
		return fmt.Errorf("load legacy station identity: %w", err)
	}
	if err := legacymigrate.Run(cfg.DataDir, cfg.ChatLog.Path, legacyCallsign); err != nil {
		return fmt.Errorf("legacy migration: %w", err)
	}
	return nil
}

func openStorageAndImport(ctx context.Context, cfg config.Config) (*storage.DB, error) {
	sqliteDB, err := storage.Open(ctx, cfg.Storage.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite storage: %w", err)
	}

	imports := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "nodes", run: func(ctx context.Context) error { return sqliteDB.ImportNodes(ctx, positions.DefaultPath(cfg.DataDir)) }},
		{name: "receive log", run: func(ctx context.Context) error { return sqliteDB.ImportReceiveLog(ctx, cfg.ReceiveLog.Path) }},
		{name: "chat history", run: func(ctx context.Context) error { return sqliteDB.ImportChatHistory(ctx, cfg.ChatLog.Path) }},
		{name: "chat reads", run: func(ctx context.Context) error {
			return sqliteDB.ImportChatReads(ctx, filepath.Join(cfg.ChatLog.Path, "msg_idx.json"))
		}},
		{name: "dm stats", run: func(ctx context.Context) error { return sqliteDB.ImportDMStats(ctx, dmstats.DefaultPath(cfg.DataDir)) }},
		{name: "stats", run: func(ctx context.Context) error { return sqliteDB.ImportStats(ctx, cfg.Stats.Path) }},
		{name: "channel show", run: func(ctx context.Context) error {
			return sqliteDB.ImportChannelShow(ctx, channelshow.DefaultPath(cfg.DataDir))
		}},
		{name: "station identity", run: func(ctx context.Context) error {
			return sqliteDB.ImportStationIdentity(ctx, station.DefaultPath(cfg.DataDir))
		}},
		{name: "http sessions", run: func(ctx context.Context) error {
			return sqliteDB.ImportHTTPSessions(ctx, filepath.Join(cfg.DataDir, "http-sessions.json"))
		}},
	}
	for _, item := range imports {
		if err := item.run(ctx); err != nil {
			_ = sqliteDB.Close()
			return nil, fmt.Errorf("import %s: %w", item.name, err)
		}
	}
	return sqliteDB, nil
}

func ensureDataDirs(dataDir string) error {
	for _, dir := range []string{"raw", "nodes", "chat", "stats", "configs"} {
		path := filepath.Join(dataDir, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create data directory %s: %w", path, err)
		}
	}

	return nil
}

func printStartupBanner(cfg config.Config, myCall string) {
	fmt.Print(startupBanner(cfg, myCall))
}

func startupBanner(cfg config.Config, myCall string) string {
	var b strings.Builder
	mode := cfg.TransportMode
	if mode == "" {
		mode = config.TransportUDP
	}
	b.WriteString(bannerRule("="))
	b.WriteString(bannerText("GOMESHCOMD"))
	b.WriteString(bannerText("MeshCom " + strings.ToUpper(mode) + " Link Terminal"))
	b.WriteString(bannerRule("-"))
	b.WriteString(bannerText("STATUS   READY"))
	b.WriteString(bannerText("VERSION  " + version))
	displayCall := myCall
	if displayCall == "" {
		displayCall = "(unset)"
	}
	b.WriteString(bannerText("MYCALL   " + displayCall))
	switch mode {
	case config.TransportSerial:
		b.WriteString(bannerText("NODE     " + cfg.Serial.Device))
		b.WriteString(bannerText("SERIAL   " + cfg.Serial.Device))
	case config.TransportNetConsole:
		b.WriteString(bannerText("NODE     " + cfg.NetConsole.Address))
		b.WriteString(bannerText("NETCON   " + cfg.NetConsole.Address))
	default:
		nodeDisplay := cfg.NodeAddr
		if nodeDisplay == "" {
			nodeDisplay = "(auto-detect from incoming UDP)"
		}
		b.WriteString(bannerText("NODE     " + nodeDisplay))
		b.WriteString(bannerText("UDP RX   " + cfg.UDPListenAddr))
	}
	b.WriteString(bannerText("HELP     gomeshcomd --help"))
	b.WriteString(bannerText("WEB UI   " + webInterfaceURL(cfg.HTTPAddr)))
	if myCall == "" {
		b.WriteString(bannerText("DMs      hidden until MyCall set"))
		if mode == config.TransportUDP {
			b.WriteString(bannerText("MSG      node addr required"))
		}
	}
	b.WriteString(bannerRule("="))
	return b.String()
}

func bannerRule(char string) string {
	return "+" + strings.Repeat(char, startupBannerInnerWidth) + "+\n"
}

func bannerText(text string) string {
	return fmt.Sprintf("| %-*s |\n", startupBannerInnerWidth-2, text)
}

func webInterfaceURL(httpAddr string) string {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return "http://" + httpAddr
	}

	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	return "http://" + net.JoinHostPort(host, port)
}

func configureLogger(levelName string) {
	levels := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	level, ok := levels[levelName]
	if !ok {
		level = slog.LevelInfo
	}

	logger := slog.New(logfmt.New(os.Stdout, level))
	slog.SetDefault(logger)
}
