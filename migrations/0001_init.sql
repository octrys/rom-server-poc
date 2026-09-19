-- +goose Up
-- +goose StatementBegin
CREATE TABLE characters (
    id         BIGSERIAL PRIMARY KEY,
    account_id BIGINT      NOT NULL,
    slot_index INTEGER     NOT NULL,
    name       TEXT        NOT NULL,
    class_type INTEGER     NOT NULL DEFAULT 0,
    level      INTEGER     NOT NULL DEFAULT 1,
    map_id     INTEGER     NOT NULL DEFAULT 0,
    x          REAL        NOT NULL DEFAULT 0,
    y          REAL        NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, slot_index)
);
CREATE INDEX characters_account_id_idx ON characters (account_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE characters;
-- +goose StatementEnd
