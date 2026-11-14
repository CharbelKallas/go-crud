-- Emails compare case-insensitively but accent-sensitively, so
-- jose@example.com and josé@example.com are different accounts.
-- tokens_revoked_at: access tokens issued at or before this time (app
-- clock, millisecond precision) are rejected; set when a stolen refresh
-- token is detected.
alter table user
    modify email varchar(255) character set utf8mb4 collate utf8mb4_0900_as_ci not null,
    add column tokens_revoked_at datetime(3) null;
