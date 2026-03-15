-- name: RecordCommand :exec
INSERT INTO command_logs (timestamp, user_email, server_id, command, response, duration_ms)
VALUES (datetime('now'), ?, ?, ?, ?, ?);

-- name: GetLogs :many
SELECT id, timestamp, user_email, server_id, command, response, duration_ms
FROM command_logs
ORDER BY timestamp DESC
LIMIT ?;

-- name: PruneOlderThan :exec
DELETE FROM command_logs
WHERE timestamp < datetime('now', ?);
