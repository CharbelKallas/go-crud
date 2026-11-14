-- Fails if accent-only variants of an email now exist (they would collide).
alter table user
    drop column tokens_revoked_at,
    modify email varchar(255) character set utf8mb4 collate utf8mb4_0900_ai_ci not null;
