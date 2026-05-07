-- Allow end_date == start_date so a one-month subscription can be expressed
-- with both fields equal (e.g. start_date = end_date = '07-2025').
ALTER TABLE subscriptions DROP CONSTRAINT end_after_start;
ALTER TABLE subscriptions
    ADD CONSTRAINT end_not_before_start
    CHECK (end_date IS NULL OR end_date >= start_date);
