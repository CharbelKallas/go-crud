alter table user
    drop column session_version,
    add column tokens_revoked_at datetime(3) null;
