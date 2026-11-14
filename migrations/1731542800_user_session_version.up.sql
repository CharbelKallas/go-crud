-- Replace the time-based revocation marker with a counter: tokens carry the
-- version current when they were issued, and revoking a user's sessions
-- increments it. No clocks are compared, so app servers with skewed clocks
-- cannot shift the revocation boundary.
alter table user
    drop column tokens_revoked_at,
    add column session_version bigint not null default 0;
