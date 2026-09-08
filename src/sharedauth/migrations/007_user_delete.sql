CREATE TRIGGER ygg_go_users_security_delete AFTER DELETE ON users
FOR EACH ROW
BEGIN
    INSERT INTO ygg_go_auth_subjects (user_id, generation, updated_at)
    VALUES (OLD.uid, 1, UTC_TIMESTAMP(6))
    ON DUPLICATE KEY UPDATE generation = generation + 1, updated_at = UTC_TIMESTAMP(6);
END
