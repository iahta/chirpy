-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;


-- name: RetrieveChirps :many
SELECT id, created_at, updated_at, body, user_id FROM chirps
ORDER BY created_at ASC;

-- name: GrabChirp :one
SELECT id, created_at, updated_at, body, user_id FROM chirps
WHERE id = $1;

-- name: DeleteChirp :exec
DELETE FROM chirps 
WHERE id = $1;

-- name: RetrieveChirpsByAuthor :many
SELECT id, created_at, updated_at, body, user_id FROM chirps
WHERE user_id = $1
ORDER BY created_at ASC;