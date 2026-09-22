-- +goose Up
UPDATE projects SET name = 'Default' WHERE name = 'Personal';
