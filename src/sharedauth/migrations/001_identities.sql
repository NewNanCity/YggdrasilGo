CREATE TABLE ygg_go_identities (
    identity_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    player_id BIGINT UNSIGNED NULL,
    uuid BINARY(16) NULL,
    state VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    legacy_mapping_id BIGINT UNSIGNED NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (identity_id),
    UNIQUE KEY uq_identity_player (player_id),
    UNIQUE KEY uq_identity_uuid (uuid),
    UNIQUE KEY uq_identity_legacy (legacy_mapping_id),
    CONSTRAINT ck_identity_player CHECK (player_id IS NULL OR player_id > 0),
    CONSTRAINT ck_identity_legacy CHECK (legacy_mapping_id IS NULL OR legacy_mapping_id > 0),
    CONSTRAINT ck_identity_state CHECK (
        (state IN ('active', 'retired') AND player_id IS NOT NULL AND uuid IS NOT NULL)
        OR (state = 'reserved' AND player_id IS NULL AND uuid IS NOT NULL)
        OR (state = 'blocked' AND player_id IS NOT NULL AND uuid IS NULL)
    )
) ENGINE=InnoDB
