-- name: UpsertTOTPSecret :exec
INSERT INTO totp_secrets (user_id, secret, verified)
VALUES ($1, $2, false)
ON CONFLICT (user_id) DO UPDATE SET secret = EXCLUDED.secret, verified = false;

-- name: GetUnverifiedTOTPSecret :one
SELECT secret FROM totp_secrets WHERE user_id = $1 AND verified = false;

-- name: GetVerifiedTOTPSecret :one
SELECT secret FROM totp_secrets WHERE user_id = $1 AND verified = true;

-- name: ConfirmTOTP :exec
UPDATE totp_secrets SET verified = true WHERE user_id = $1;

-- name: DisableTOTP :exec
DELETE FROM totp_secrets WHERE user_id = $1;
