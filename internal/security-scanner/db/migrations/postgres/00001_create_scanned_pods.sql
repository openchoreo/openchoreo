-- +goose Up
-- +goose StatementBegin
CREATE TABLE scanned_pods (
    id SERIAL PRIMARY KEY,
    pod_name TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE scanned_pods;
-- +goose StatementEnd
