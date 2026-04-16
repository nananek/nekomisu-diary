-- name: ListWebAuthnCredentials :many
SELECT id, name, created_at
FROM webauthn_credentials
WHERE user_id = $1
ORDER BY created_at;

-- name: LoadWebAuthnCredentials :many
SELECT id, public_key, attestation_type, sign_count, transports,
       flag_backup_eligible, flag_backup_state
FROM webauthn_credentials WHERE user_id = $1;

-- name: CreateWebAuthnCredential :exec
INSERT INTO webauthn_credentials
  (id, user_id, name, public_key, attestation_type, sign_count,
   transports, flag_backup_eligible, flag_backup_state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateWebAuthnSignCount :exec
UPDATE webauthn_credentials SET sign_count = $1 WHERE id = $2;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2;
