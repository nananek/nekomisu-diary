-- +goose Up
-- WebAuthn authenticators mark credentials with Backup Eligible /
-- Backup State flags at registration. go-webauthn validates these on
-- every login and rejects mismatches. Before this migration, the columns
-- weren't persisted, so any credential registered on an authenticator
-- that supports backup (iCloud Keychain / Google Password Manager
-- synced passkeys) would fail to log in with:
--   "Backup Eligible flag inconsistency detected during login validation"

ALTER TABLE webauthn_credentials
  ADD COLUMN IF NOT EXISTS flag_backup_eligible BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS flag_backup_state    BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE webauthn_credentials
  DROP COLUMN IF EXISTS flag_backup_eligible,
  DROP COLUMN IF EXISTS flag_backup_state;
