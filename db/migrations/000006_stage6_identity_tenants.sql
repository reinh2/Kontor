-- Kontor Stage 6: multi-tenancy, operator identity, and channel configuration.
--
-- Every domain row has carried tenant_id since Stage 1.  This migration adds
-- the missing identity boundary and the mutable tenant-owned channel settings
-- needed to operate more than the original demo tenant safely.

-- Currency was previously a fixed process setting. It becomes tenant-owned so
-- operator sessions and catalogue data can describe two businesses correctly.
ALTER TABLE tenants
    ADD COLUMN currency char(3) NOT NULL DEFAULT 'EUR'
    CHECK (currency = upper(currency));

CREATE TABLE operators (
    tenant_id     uuid NOT NULL,
    id            uuid NOT NULL DEFAULT gen_random_uuid(),
    email         text NOT NULL CHECK (
        email = lower(email)
        AND length(email) BETWEEN 3 AND 254
        AND position('@' IN email) > 1
    ),
    display_name  text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    password_hash text NOT NULL CHECK (length(password_hash) BETWEEN 60 AND 255),
    role          text NOT NULL CHECK (role IN ('owner', 'staff')),
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
);

-- Email is a login name inside its tenant.  The expression index keeps the
-- database invariant even if a future import path bypasses application input
-- normalization.
CREATE UNIQUE INDEX operators_tenant_email_uq
    ON operators (tenant_id, lower(email));
CREATE INDEX operators_tenant_active_idx
    ON operators (tenant_id, active, display_name, id);

CREATE TABLE operator_sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL,
    operator_id  uuid NOT NULL,
    token_digest bytea NOT NULL CHECK (octet_length(token_digest) = 32),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    last_used_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, operator_id)
        REFERENCES operators (tenant_id, id) ON DELETE CASCADE,
    CHECK (expires_at > created_at)
);

-- Only SHA-256 digests of opaque session credentials are persisted.  A leaked
-- database cannot replay an operator session without the browser-held token.
CREATE UNIQUE INDEX operator_sessions_token_digest_uq
    ON operator_sessions (token_digest);
CREATE INDEX operator_sessions_validate_idx
    ON operator_sessions (token_digest, expires_at)
    WHERE revoked_at IS NULL;
CREATE INDEX operator_sessions_operator_idx
    ON operator_sessions (tenant_id, operator_id, expires_at)
    WHERE revoked_at IS NULL;

CREATE TABLE tenant_channels (
    tenant_id                          uuid PRIMARY KEY,
    widget_origin                      text NOT NULL DEFAULT '' CHECK (length(widget_origin) <= 2048),
    telegram_bot_token_ciphertext       bytea,
    telegram_bot_token_nonce            bytea,
    telegram_webhook_secret_digest      bytea CHECK (
        telegram_webhook_secret_digest IS NULL
        OR octet_length(telegram_webhook_secret_digest) = 32
    ),
    telegram_enabled                    boolean NOT NULL DEFAULT false,
    created_at                          timestamptz NOT NULL DEFAULT now(),
    updated_at                          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE,
    CHECK (
        (telegram_bot_token_ciphertext IS NULL) = (telegram_bot_token_nonce IS NULL)
    ),
    CHECK (
        NOT telegram_enabled OR (
            telegram_bot_token_ciphertext IS NOT NULL
            AND telegram_webhook_secret_digest IS NOT NULL
        )
    )
);

-- Make the Stage 1 tenant configurable without changing its identity. New
-- tenants are provisioned with their row transactionally by the Stage 6
-- onboarding service.
INSERT INTO tenant_channels (tenant_id)
SELECT id FROM tenants
ON CONFLICT (tenant_id) DO NOTHING;

COMMENT ON TABLE operators IS
    'Tenant-local human operator identities. Roles are owner or staff; all operator API requests authenticate through operator_sessions.';
COMMENT ON TABLE operator_sessions IS
    'Opaque bearer sessions. token_digest is SHA-256(raw token); raw tokens are returned once at login and never stored.';
COMMENT ON TABLE tenant_channels IS
    'Per-tenant widget and Telegram configuration. Telegram bot token ciphertext is encrypted by the application; webhook secrets are digest-only.';
