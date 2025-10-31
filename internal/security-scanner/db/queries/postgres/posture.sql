-- name: GetPostureScannedResource :one
SELECT psr.* 
FROM posture_scanned_resources psr
JOIN resources r ON psr.resource_id = r.id
WHERE r.resource_type = $1 AND r.resource_namespace = $2 AND r.resource_name = $3;

-- name: UpsertPostureScannedResource :exec
INSERT INTO posture_scanned_resources (resource_id, resource_version, scan_duration_ms, scanned_at)
VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
ON CONFLICT(resource_id) DO UPDATE SET 
  resource_version = EXCLUDED.resource_version,
  scan_duration_ms = EXCLUDED.scan_duration_ms,
  scanned_at = CURRENT_TIMESTAMP;

-- name: InsertPostureFinding :exec
INSERT INTO posture_findings (resource_id, check_id, check_name, severity, category, description, remediation, resource_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: DeletePostureFindingsByResourceID :exec
DELETE FROM posture_findings WHERE resource_id = $1;

-- name: ListPostureFindings :many
SELECT f.*, r.resource_type, r.resource_namespace, r.resource_name
FROM posture_findings f
JOIN resources r ON f.resource_id = r.id
ORDER BY f.created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetPostureFindingsByResourceID :many
SELECT * FROM posture_findings WHERE resource_id = $1 ORDER BY created_at DESC;

-- name: ListResourcesWithPostureFindings :many
SELECT DISTINCT r.*
FROM resources r
JOIN posture_findings f ON r.id = f.resource_id
ORDER BY r.updated_at DESC
LIMIT $1 OFFSET $2;

-- name: CountResourcesWithPostureFindings :one
SELECT COUNT(DISTINCT r.id) as count
FROM resources r
JOIN posture_findings f ON r.id = f.resource_id;
