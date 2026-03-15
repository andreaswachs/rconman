CREATE TABLE IF NOT EXISTS command_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_email TEXT NOT NULL,
    server_id TEXT NOT NULL,
    command TEXT NOT NULL,
    response TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_command_logs_timestamp ON command_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_command_logs_server_id ON command_logs(server_id);
CREATE INDEX IF NOT EXISTS idx_command_logs_user_email ON command_logs(user_email);
