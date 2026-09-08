CREATE TABLE ygg_go_join_sessions (
    server_id VARBINARY(255) NOT NULL,
    token_hash BINARY(32) NOT NULL,
    client_ip VARBINARY(16) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (server_id),
    KEY ix_session_token (token_hash),
    KEY ix_session_expiry (expires_at, server_id),
    CONSTRAINT ck_session_server CHECK (OCTET_LENGTH(server_id) > 0),
    CONSTRAINT ck_session_ip CHECK (OCTET_LENGTH(client_ip) IN (4, 16)),
    CONSTRAINT ck_session_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB
