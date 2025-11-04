-- name: UpsertResource :one
INSERT INTO resources (resource_type, resource_namespace, resource_name, resource_uid, resource_version)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(resource_type, resource_namespace, resource_name) 
DO UPDATE SET 
  resource_version = excluded.resource_version,
  resource_uid = excluded.resource_uid,
  updated_at = CURRENT_TIMESTAMP
RETURNING id;

-- name: InsertResourceLabel :exec
INSERT INTO resource_labels (resource_id, label_key, label_value)
VALUES (?, ?, ?)
ON CONFLICT(resource_id, label_key) DO UPDATE SET label_value = excluded.label_value;

-- name: DeleteResourceLabels :exec
DELETE FROM resource_labels WHERE resource_id = ?;

-- name: GetResource :one
SELECT * FROM resources WHERE id = ?;

-- name: GetResourceLabels :many
SELECT label_key, label_value FROM resource_labels WHERE resource_id = ?;
