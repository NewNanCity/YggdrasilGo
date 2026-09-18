CREATE TRIGGER ygg_go_identity_resolution_insert
AFTER INSERT ON ygg_go_identities
FOR EACH ROW
BEGIN
    IF NEW.state = 'resolved'
        AND NEW.resolved_into_identity_id = NEW.identity_id THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'identity cannot resolve into itself';
    END IF;
END
