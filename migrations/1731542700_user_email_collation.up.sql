-- Emails compare case-insensitively but accent-sensitively, so
-- jose@example.com and josé@example.com are different accounts.
alter table user
    modify email varchar(255) character set utf8mb4 collate utf8mb4_0900_as_ci not null;
