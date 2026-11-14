-- The token issued when this one was used. A retry within the grace window
-- gets that same token back instead of a new one, so a replayed token never
-- yields an independent, undetectable token chain.
alter table refresh_token add column replaced_by char(32) null;
