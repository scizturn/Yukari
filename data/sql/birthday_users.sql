SELECT
  user_id,
  name,
  email,
  birthdate,
  TRUE AS is_active
FROM users
WHERE birthdate IS NOT NULL
  AND DATE_FORMAT(birthdate, '%m-%d') = ?
  AND email IS NOT NULL
  AND email <> '';
