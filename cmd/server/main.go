// Command server is the ROM game-server POC: it accepts raw-TCP game clients on
// the game port, performs the encrypted handshake, authenticates each session
// against rom-api, and dispatches messages to the world simulation.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/octrys/rom-server-poc/internal/auth"
	"github.com/octrys/rom-server-poc/internal/config"
	"github.com/octrys/rom-server-poc/internal/game"
	"github.com/octrys/rom-server-poc/internal/persist/postgres"
	"github.com/octrys/rom-server-poc/internal/transport"
	"github.com/octrys/rom-server-poc/internal/world"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	logger.Info("connected to database")

	authClient := auth.NewClient(cfg.RegionInternalURL, cfg.ConsumeSession, cfg.AuthTimeout)

	gameWorld := world.New(cfg.TickInterval, logger)
	go gameWorld.Run(ctx)

	handler := &game.Handler{
		Auth:    authClient,
		Store:   store,
		World:   gameWorld,
		Logger:  logger,
		WorldID: cfg.WorldID,
	}

	listener, err := net.Listen("tcp", cfg.GameListenAddr)
	if err != nil {
		return err
	}
	logger.Info("game server listening", "addr", cfg.GameListenAddr, "world", cfg.WorldID)

	// Close the listener when the context is cancelled so Accept unblocks.
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	for {
		raw, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			logger.Warn("accept failed", "error", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveConn(ctx, handler, raw, logger)
		}()
	}

	logger.Info("shutting down; waiting for connections to drain")
	wg.Wait()
	return nil
}

// serveConn performs the handshake for one accepted socket and hands it to the
// game handler.
func serveConn(ctx context.Context, handler *game.Handler, raw net.Conn, logger *slog.Logger) {
	conn, err := transport.Handshake(raw)
	if err != nil {
		logger.Warn("handshake failed", "peer", raw.RemoteAddr().String(), "error", err)
		_ = raw.Close()
		return
	}
	handler.Serve(ctx, conn)
}
