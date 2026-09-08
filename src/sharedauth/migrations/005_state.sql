CREATE TABLE ygg_go_state (
    id TINYINT UNSIGNED NOT NULL,
    schema_version INT UNSIGNED NOT NULL,
    phase VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    player_high_watermark BIGINT UNSIGNED NOT NULL,
    migration_id BINARY(16) NOT NULL,
    activated_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    CONSTRAINT ck_state_singleton CHECK (id = 1),
    CONSTRAINT ck_state_phase CHECK (
        (phase = 'staged' AND activated_at IS NULL)
        OR (phase = 'active' AND activated_at IS NOT NULL)
    )
) ENGINE=InnoDB
