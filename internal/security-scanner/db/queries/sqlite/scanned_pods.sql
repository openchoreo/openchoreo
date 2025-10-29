-- name: InsertScannedPod :exec
INSERT INTO scanned_pods (pod_name) VALUES (?);

-- name: GetScannedPod :one
SELECT * FROM scanned_pods WHERE id = ?;

-- name: GetScannedPodByName :one
SELECT * FROM scanned_pods WHERE pod_name = ?;

-- name: ListScannedPods :many
SELECT * FROM scanned_pods;

-- name: DeleteScannedPod :exec
DELETE FROM scanned_pods WHERE id = ?;
