-- +goose Up
CREATE UNIQUE INDEX application_pending_unique ON application (candidate_id) WHERE state = 'pending';

-- +goose Down
DROP INDEX application_pending_unique;
