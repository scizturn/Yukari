SELECT
  user_id,
  name,
  email,
  birthdate,
  is_confirmed
FROM users
WHERE birthdate IS NOT NULL
  AND DATE_FORMAT(birthdate, '%m-%d') = ?
  AND is_confirmed = 1
  AND email IS NOT NULL
  AND email <> '';
