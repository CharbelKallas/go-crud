-- Tokens carry the session version current when they were issued; revoking
-- a user's sessions increments it, which invalidates every older token
-- without comparing clocks.
alter table user add column session_version bigint not null default 0;
