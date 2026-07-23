-- The widget accepts an optional contact value and the application collects a
-- literal email or phone later from the authenticated customer's message.
-- Keep customers without contact details valid until that collection occurs.
ALTER TABLE customers DROP CONSTRAINT IF EXISTS customers_check;
