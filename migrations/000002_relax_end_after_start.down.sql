ALTER TABLE subscriptions DROP CONSTRAINT end_not_before_start;
ALTER TABLE subscriptions
    ADD CONSTRAINT end_after_start
    CHECK (end_date IS NULL OR end_date > start_date);
