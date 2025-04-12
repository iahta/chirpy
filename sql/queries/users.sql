-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;

-- name: DeleteUsers :exec
DELETE FROM users;


-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1;


-- name: GetUserById :one
SELECT * FROM users
WHERE id = $1;

-- name: UpdatePasswordEmailUser :one
UPDATE users
SET email = $1, hashed_password = $2, updated_at = $3
WHERE id = $4
RETURNING id, created_at, updated_at, email, is_chirpy_red;
 
-- name: UpgradeUserToRed :exec
UPDATE users
SET is_chirpy_red = true
WHERE id = $1;

-- name: IsUserChirpyRed :one
SELECT is_chirpy_red FROM users
WHERE id = $1;