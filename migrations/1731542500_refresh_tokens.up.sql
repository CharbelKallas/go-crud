-- One row per issued refresh token, so each can be used once and all of a
-- user's tokens can be revoked. Times are UTC.
create table if not exists refresh_token
(
    jti        char(32) not null primary key,
    user_id    int      not null,
    expires_at datetime not null,
    used_at    datetime null,
    revoked_at datetime null,
    index refresh_token_user (user_id),
    index refresh_token_expires (expires_at),
    constraint refresh_token_user_fk foreign key (user_id) references user (id) on delete cascade
);
