-- +goose Up

-- Where a reply should go when it is not the sender: an invoice is sent from
-- the platform's verified address, but the customer should answer the DJ.
-- Stored with the message so a retry sends exactly what the first attempt would.
-- Empty means no reply-to.
ALTER TABLE email_messages ADD COLUMN reply_to TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE email_messages DROP COLUMN reply_to;
