SELECT
  user_id,
  name,
  email,
  birthdate,
  email_verified_at IS NOT NULL AS is_active
FROM users
WHERE birthdate IS NOT NULL
  AND DATE_FORMAT(birthdate, '%m-%d') = ?
  AND email_verified_at IS NOT NULL
  AND email IS NOT NULL
  AND email <> '';
