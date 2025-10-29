-- +goose Up
-- +goose StatementBegin
CREATE TABLE resources (
  id SERIAL PRIMARY KEY,
  resource_type TEXT NOT NULL,
  resource_namespace TEXT NOT NULL,
  resource_name TEXT NOT NULL,
  resource_uid TEXT NOT NULL,
  resource_version TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_type, resource_namespace, resource_name)
);

CREATE INDEX idx_resources_lookup ON resources(resource_type, resource_namespace, resource_name);
CREATE INDEX idx_resources_uid ON resources(resource_uid);

CREATE TABLE resource_labels (
  id SERIAL PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  label_key TEXT NOT NULL,
  label_value TEXT NOT NULL,
  UNIQUE(resource_id, label_key),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE INDEX idx_resource_labels_lookup ON resource_labels(label_key, label_value);
CREATE INDEX idx_resource_labels_resource_id ON resource_labels(resource_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_resource_labels_resource_id;
DROP INDEX IF EXISTS idx_resource_labels_lookup;
DROP TABLE IF EXISTS resource_labels;
DROP INDEX IF EXISTS idx_resources_uid;
DROP INDEX IF EXISTS idx_resources_lookup;
DROP TABLE IF EXISTS resources;
-- +goose StatementEnd
