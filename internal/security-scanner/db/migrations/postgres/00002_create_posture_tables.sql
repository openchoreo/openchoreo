-- +goose Up
-- +goose StatementBegin
CREATE TABLE posture_scanned_resources (
  id SERIAL PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  resource_version TEXT NOT NULL,
  scan_duration_ms INTEGER,
  scanned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_id),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE INDEX idx_posture_scanned_resources_resource_id ON posture_scanned_resources(resource_id);

CREATE TABLE posture_findings (
  id SERIAL PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  check_id TEXT NOT NULL,
  check_name TEXT NOT NULL,
  severity TEXT NOT NULL,
  category TEXT,
  description TEXT,
  remediation TEXT,
  resource_version TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE INDEX idx_posture_findings_resource_id ON posture_findings(resource_id);
CREATE INDEX idx_posture_findings_severity ON posture_findings(severity);
CREATE INDEX idx_posture_findings_check_id ON posture_findings(check_id);
CREATE INDEX idx_posture_findings_category ON posture_findings(category);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_posture_findings_category;
DROP INDEX IF EXISTS idx_posture_findings_check_id;
DROP INDEX IF EXISTS idx_posture_findings_severity;
DROP INDEX IF EXISTS idx_posture_findings_resource_id;
DROP TABLE IF EXISTS posture_findings;
DROP INDEX IF EXISTS idx_posture_scanned_resources_resource_id;
DROP TABLE IF EXISTS posture_scanned_resources;
-- +goose StatementEnd
