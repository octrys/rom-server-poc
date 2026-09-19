// Package postgres is the Postgres-backed implementation of persist.Store, using
// pgx's connection pool.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/octrys/rom-server-poc/internal/persist"
)

// Store implements persist.Store over a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New opens a connection pool against the given DSN and verifies connectivity.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

func (s *Store) UpsertAccount(ctx context.Context, accountID int64, userCode string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO accounts (account_id, user_code, last_login_at)
		VALUES ($1, $2, now())
		ON CONFLICT (account_id) DO UPDATE
			SET user_code = EXCLUDED.user_code, last_login_at = now()`,
		accountID, userCode)
	if err != nil {
		return fmt.Errorf("postgres: upsert account: %w", err)
	}
	return nil
}

func (s *Store) ListCharacters(ctx context.Context, accountID int64) ([]persist.Character, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, slot_index, name, class_type, level, map_id, x, y, created_at
		FROM characters
		WHERE account_id = $1
		ORDER BY slot_index`, accountID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list characters: %w", err)
	}
	defer rows.Close()

	var characters []persist.Character
	for rows.Next() {
		c, err := scanCharacter(rows)
		if err != nil {
			return nil, err
		}
		characters = append(characters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list characters: %w", err)
	}
	return characters, nil
}

func (s *Store) GetCharacter(ctx context.Context, id int64) (persist.Character, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, account_id, slot_index, name, class_type, level, map_id, x, y, created_at
		FROM characters
		WHERE id = $1`, id)
	c, err := scanCharacter(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return persist.Character{}, persist.ErrNotFound
	}
	if err != nil {
		return persist.Character{}, fmt.Errorf("postgres: get character: %w", err)
	}
	return c, nil
}

func (s *Store) CreateCharacter(ctx context.Context, c persist.Character) (persist.Character, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO characters (account_id, slot_index, name, class_type, level, map_id, x, y)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		c.AccountID, c.SlotIndex, c.Name, c.ClassType, c.Level, c.MapID, c.X, c.Y)
	if err := row.Scan(&c.ID, &c.CreatedAt); err != nil {
		return persist.Character{}, fmt.Errorf("postgres: create character: %w", err)
	}
	return c, nil
}

func (s *Store) SavePosition(ctx context.Context, id int64, mapID int32, x, y float32) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE characters SET map_id = $2, x = $3, y = $4 WHERE id = $1`, id, mapID, x, y)
	if err != nil {
		return fmt.Errorf("postgres: save position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return persist.ErrNotFound
	}
	return nil
}

// scanner abstracts over pgx.Row and pgx.Rows for a single row scan.
type scanner interface {
	Scan(dest ...any) error
}

func scanCharacter(row scanner) (persist.Character, error) {
	var c persist.Character
	err := row.Scan(&c.ID, &c.AccountID, &c.SlotIndex, &c.Name, &c.ClassType,
		&c.Level, &c.MapID, &c.X, &c.Y, &c.CreatedAt)
	return c, err
}
