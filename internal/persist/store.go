// Package persist defines durable game state and the Store abstraction over it.
// Hot mutable state (position, HP) lives in the world tick loop; the Store holds
// only what must survive a restart. The concrete Postgres implementation is in
// the postgres subpackage.
package persist

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("persist: not found")

// Account is the game server's local projection of an account. Identity is owned
// by rom-api; this row holds only game-side state (options, last login) keyed by
// the auth-owned AccountID.
type Account struct {
	AccountID    int64
	UserCode     string
	OptionValues int32
	CreatedAt    time.Time
}

// Character is a persisted player character, owned by an account (whose identity
// lives in rom-api, referenced here only by AccountID).
type Character struct {
	ID        int64
	AccountID int64
	SlotIndex int32
	Name      string
	ClassType int32
	Level     int32
	MapID     int32
	X         float32
	Y         float32
	CreatedAt time.Time
}

// Store is the persistence boundary. Implementations must be safe for concurrent
// use by multiple sessions.
type Store interface {
	// UpsertAccount creates the local account row on first login and refreshes
	// its cached user code on every login. Call it once a session is validated,
	// before any character operation (characters reference the account).
	UpsertAccount(ctx context.Context, accountID int64, userCode string) error
	// ListCharacters returns an account's characters, ordered by slot.
	ListCharacters(ctx context.Context, accountID int64) ([]Character, error)
	// GetCharacter returns one character by id, or ErrNotFound.
	GetCharacter(ctx context.Context, id int64) (Character, error)
	// CreateCharacter inserts a new character and returns it with its assigned ID.
	CreateCharacter(ctx context.Context, c Character) (Character, error)
	// SavePosition persists a character's map and coordinates (periodic snapshot).
	SavePosition(ctx context.Context, id int64, mapID int32, x, y float32) error
	// Close releases any underlying resources.
	Close()
}
