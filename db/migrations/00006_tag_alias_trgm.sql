-- +goose Up
CREATE INDEX tag_alias_normalized_trgm_gin ON tag_alias USING gin (normalized gin_trgm_ops);

-- +goose Down
DROP INDEX tag_alias_normalized_trgm_gin;
