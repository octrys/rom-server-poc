-- +goose Up
-- +goose StatementBegin
CREATE TABLE accounts (
    account_id    BIGINT      PRIMARY KEY,          -- owned by rom-api; not generated here
    user_code     TEXT        NOT NULL,             -- cached from the validated session
    option_values INTEGER     NOT NULL DEFAULT 0,   -- C2S_SetOptions m_optionValues
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE characters
    ADD CONSTRAINT characters_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts (account_id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE characters DROP CONSTRAINT characters_account_id_fkey;
DROP TABLE accounts;
-- +goose StatementEnd
