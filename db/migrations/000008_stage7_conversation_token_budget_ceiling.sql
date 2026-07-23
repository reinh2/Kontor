-- A real-model booking spends ~65 000 tokens across two turns, and the
-- conservative estimator reserves roughly three times a turn's actual use on
-- top of that, so the original 100 000 ceiling could not hold one completed
-- booking. Raise the ceiling; the per-conversation cap itself is unchanged and
-- still enforced by tokens_used + tokens_reserved <= token_budget.
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversations_token_budget_check;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_token_budget_check
    CHECK (token_budget BETWEEN 1 AND 2000000);
