CREATE TABLE ygg_go_auth_subjects (
    user_id BIGINT UNSIGNED NOT NULL,
    generation BIGINT UNSIGNED NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (user_id),
    CONSTRAINT ck_subject_user CHECK (user_id > 0),
    CONSTRAINT ck_subject_generation CHECK (generation >= 1)
) ENGINE=InnoDB
