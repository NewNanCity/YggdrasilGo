CREATE TRIGGER ygg_go_users_security_update AFTER UPDATE ON users
FOR EACH ROW
BEGIN
    IF NOT (CAST(OLD.password AS BINARY) <=> CAST(NEW.password AS BINARY))
        OR NOT (OLD.permission <=> NEW.permission) THEN
        INSERT INTO ygg_go_auth_subjects (user_id, generation, updated_at)
        VALUES (OLD.uid, 1, UTC_TIMESTAMP(6))
        ON DUPLICATE KEY UPDATE generation = generation + 1, updated_at = UTC_TIMESTAMP(6);
    END IF;
END
