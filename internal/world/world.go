// Package world holds authoritative, in-memory game state and steps the
// simulation. All mutable state is owned by a single goroutine (Run); other
// goroutines interact only by enqueuing commands, so no locks guard game state.
package world

import (
	"context"
	"log/slog"
	"time"

	"github.com/octrys/rom-server-poc/internal/protocol"
)

// Sender delivers a message to a connected client. transport.Conn satisfies it.
type Sender interface {
	Send(name string, values protocol.Values) error
}

// Player is an entity present in the world, bound to a live connection.
type Player struct {
	OID         int64 // world-unique object id sent to clients
	CharacterID int64
	AccountID   int64
	Name        string
	MapID       int32
	X, Y, Z     float32
	Dir         float32
	Conn        Sender
}

// World is the simulation. Construct with New, then run Run in its own goroutine.
type World struct {
	tick   time.Duration
	cmds   chan func(*World)
	logger *slog.Logger

	// state below is touched only from the Run goroutine.
	players map[int64]*Player
}

// New creates a world that steps every tick interval.
func New(tick time.Duration, logger *slog.Logger) *World {
	return &World{
		tick:    tick,
		cmds:    make(chan func(*World), 256),
		logger:  logger,
		players: make(map[int64]*Player),
	}
}

// Run owns and mutates world state until ctx is cancelled: it drains commands
// and advances the simulation each tick. Call it in its own goroutine.
func (w *World) Run(ctx context.Context) {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case fn := <-w.cmds:
			fn(w)
		case <-ticker.C:
			w.step()
		}
	}
}

// step advances the simulation one tick. Placeholder for movement resolution,
// AI, and state broadcast — the point where real game logic will grow.
func (w *World) step() {
	// TODO: resolve movement, run monster AI, broadcast deltas to nearby players.
}

// do enqueues a command for the Run goroutine, dropping it if ctx is cancelled.
func (w *World) do(ctx context.Context, fn func(*World)) {
	select {
	case w.cmds <- fn:
	case <-ctx.Done():
	}
}

// Join adds a player to the world.
func (w *World) Join(ctx context.Context, p *Player) {
	w.do(ctx, func(world *World) {
		world.players[p.CharacterID] = p
		world.logger.Info("player joined", "character", p.Name, "map", p.MapID)
	})
}

// Leave removes a player from the world.
func (w *World) Leave(ctx context.Context, characterID int64) {
	w.do(ctx, func(world *World) {
		if p, ok := world.players[characterID]; ok {
			delete(world.players, characterID)
			world.logger.Info("player left", "character", p.Name)
		}
	})
}

// Move updates a player's pose (echo movement, for now).
func (w *World) Move(ctx context.Context, characterID int64, x, y, z, dir float32) {
	w.do(ctx, func(world *World) {
		if p, ok := world.players[characterID]; ok {
			p.X, p.Y, p.Z, p.Dir = x, y, z, dir
		}
	})
}
