CREATE TABLE ygg_go_tokens (
    token_hash BINARY(32) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    generation BIGINT UNSIGNED NOT NULL,
    identity_id BIGINT UNSIGNED NULL,
    client_token MEDIUMBLOB NOT NULL,
    created_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (token_hash),
    KEY ix_token_user_generation_expiry (user_id, generation, expires_at),
    KEY ix_token_expiry (expires_at, token_hash),
    KEY ix_token_identity (identity_id),
    CONSTRAINT fk_token_subject FOREIGN KEY (user_id)
        REFERENCES ygg_go_auth_subjects (user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_token_identity FOREIGN KEY (identity_id)
        REFERENCES ygg_go_identities (identity_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT ck_token_generation CHECK (generation >= 1),
    CONSTRAINT ck_token_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB
