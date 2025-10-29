-- name: UpsertResource :one
INSERT INTO resources (resource_type, resource_namespace, resource_name, resource_uid, resource_version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(resource_type, resource_namespace, resource_name) 
DO UPDATE SET 
  resource_version = EXCLUDED.resource_version,
  resource_uid = EXCLUDED.resource_uid,
  updated_at = CURRENT_TIMESTAMP
RETURNING id;

-- name: InsertResourceLabel :exec
INSERT INTO resource_labels (resource_id, label_key, label_value)
VALUES ($1, $2, $3)
ON CONFLICT(resource_id, label_key) DO UPDATE SET label_value = EXCLUDED.label_value;

-- name: DeleteResourceLabels :exec
DELETE FROM resource_labels WHERE resource_id = $1;
