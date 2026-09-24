-- +goose Up
ALTER TABLE projects DROP COLUMN environment_id;
ALTER TABLE projects DROP COLUMN isolation;
DROP TABLE environments;
