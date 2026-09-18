ALTER TABLE ygg_go_identities
    DROP FOREIGN KEY fk_identity_resolved_into,
    DROP INDEX ix_identity_resolved_into,
    DROP CHECK ck_identity_state,
    ADD CONSTRAINT ck_identity_state CHECK (
        (state IN ('active', 'retired') AND player_id IS NOT NULL AND uuid IS NOT NULL)
        OR (state = 'reserved' AND player_id IS NULL AND uuid IS NOT NULL)
        OR (state = 'blocked' AND player_id IS NOT NULL AND uuid IS NULL)
    ),
    DROP COLUMN resolved_into_identity_id
