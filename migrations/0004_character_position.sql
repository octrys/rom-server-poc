-- +goose Up
-- +goose StatementBegin
-- The world is 3D and entities carry a facing angle: persist the full spawn
-- pose (z height and m_dir) alongside the existing x/y so a character re-enters
-- where it left off.
ALTER TABLE characters
    ADD COLUMN z   REAL NOT NULL DEFAULT 0,
    ADD COLUMN dir REAL NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE characters
    DROP COLUMN z,
    DROP COLUMN dir;
-- +goose StatementEnd
