-- name: ListMatchesByUser :many
SELECT m.id, m.state, m.created_at,
       u.id AS other_id, u.telegram_user_id AS other_telegram_user_id,
       u.telegram_username AS other_telegram_username,
       p.first_name AS other_first_name, p.last_name AS other_last_name,
       p.location AS other_location, p.meeting_format AS other_meeting_format
FROM match m
JOIN users u ON u.id = CASE WHEN m.user_low = $1 THEN m.user_high ELSE m.user_low END
LEFT JOIN profile p ON p.user_id = u.id
WHERE m.user_low = $1 OR m.user_high = $1
ORDER BY m.created_at DESC;
