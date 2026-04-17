-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, verified, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetSession :one
SELECT s.user_id, s.expires_at, u.login, u.display_name, u.avatar_path,
       EXISTS(SELECT 1 FROM totp_secrets WHERE user_id = u.id AND verified = true) AS has_totp,
       EXISTS(SELECT 1 FROM webauthn_credentials WHERE user_id = u.id) AS has_webauthn
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > NOW() AND s.verified = $2;

-- name: VerifySession :exec
UPDATE sessions SET verified = true, expires_at = $1 WHERE id = $2;

-- name: ExtendSessionExpiry :exec
UPDATE sessions SET expires_at = $1 WHERE id = $2;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < NOW();
