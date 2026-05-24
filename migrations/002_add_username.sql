ALTER TABLE users ADD COLUMN username VARCHAR(255) NOT NULL DEFAULT '';
UPDATE users SET username = email;
ALTER TABLE users ALTER COLUMN username DROP DEFAULT;
CREATE UNIQUE INDEX idx_users_username ON users(username);
