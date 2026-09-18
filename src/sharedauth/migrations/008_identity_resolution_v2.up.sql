ALTER TABLE ygg_go_identities
    ADD COLUMN resolved_into_identity_id BIGINT UNSIGNED NULL AFTER legacy_mapping_id,
    ADD KEY ix_identity_resolved_into (resolved_into_identity_id),
    ADD CONSTRAINT fk_identity_resolved_into FOREIGN KEY (resolved_into_identity_id)
        REFERENCES ygg_go_identities (identity_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    DROP CHECK ck_identity_state,
    ADD CONSTRAINT ck_identity_state CHECK (
        (state IN ('active', 'retired') AND player_id IS NOT NULL AND uuid IS NOT NULL
            AND resolved_into_identity_id IS NULL)
        OR (state = 'reserved' AND player_id IS NULL AND uuid IS NOT NULL
            AND resolved_into_identity_id IS NULL)
        OR (state = 'blocked' AND player_id IS NOT NULL AND uuid IS NULL
            AND resolved_into_identity_id IS NULL)
		OR (state = 'resolved' AND player_id IS NULL AND uuid IS NULL
			AND legacy_mapping_id IS NULL AND resolved_into_identity_id IS NOT NULL
		)
	)
