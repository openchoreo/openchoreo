-- name: InsertScannedPod :exec
INSERT INTO scanned_pods (pod_name) VALUES ($1);

-- name: GetScannedPod :one
SELECT * FROM scanned_pods WHERE id = $1;

-- name: GetScannedPodByName :one
SELECT * FROM scanned_pods WHERE pod_name = $1;

-- name: ListScannedPods :many
SELECT * FROM scanned_pods;

-- name: DeleteScannedPod :exec
DELETE FROM scanned_pods WHERE id = $1;
