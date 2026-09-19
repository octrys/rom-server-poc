-- +goose Up
-- +goose StatementBegin
ALTER TABLE characters
    ADD COLUMN sub_class_type        INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN head_type             INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN exp                   BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN real_power            INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN equip_costume_index   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN equip_costume_step_up INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN weapon_item_index     INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN weapon_enchant        INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN guild_name            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN latest_login_time     BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN latest_logout_time    BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN deleted_time          BIGINT  NOT NULL DEFAULT 0;

-- m_optionValues is a UInt32 bitmask, which can exceed INTEGER's range.
ALTER TABLE accounts ALTER COLUMN option_values TYPE BIGINT;

-- Character names are globally unique among non-deleted characters.
CREATE UNIQUE INDEX characters_name_active_key ON characters (name) WHERE deleted_time = 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX characters_name_active_key;
ALTER TABLE accounts ALTER COLUMN option_values TYPE INTEGER;
ALTER TABLE characters
    DROP COLUMN sub_class_type,
    DROP COLUMN head_type,
    DROP COLUMN exp,
    DROP COLUMN real_power,
    DROP COLUMN equip_costume_index,
    DROP COLUMN equip_costume_step_up,
    DROP COLUMN weapon_item_index,
    DROP COLUMN weapon_enchant,
    DROP COLUMN guild_name,
    DROP COLUMN latest_login_time,
    DROP COLUMN latest_logout_time,
    DROP COLUMN deleted_time;
-- +goose StatementEnd
