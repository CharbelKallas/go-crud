-- One account per email. Fails if duplicate emails already exist: remove
-- them first (see README).
alter table user add constraint user_email_unique unique (email);
