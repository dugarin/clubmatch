-- +goose Up
CREATE TYPE evaluation_value AS ENUM ('interested', 'not_interested');
CREATE TYPE evaluation_source AS ENUM ('deck', 'search');
CREATE TYPE match_state AS ENUM ('active', 'contact_unavailable');
CREATE TYPE application_state AS ENUM ('pending', 'approved', 'rejected');

CREATE TABLE evaluation (
    evaluator_id uuid NOT NULL REFERENCES users (id),
    target_id    uuid NOT NULL REFERENCES users (id),
    value        evaluation_value NOT NULL,
    source       evaluation_source NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (evaluator_id, target_id),
    CHECK (evaluator_id <> target_id)
);

CREATE INDEX evaluation_target_id_idx ON evaluation (target_id);
CREATE INDEX evaluation_evaluator_updated_idx ON evaluation (evaluator_id, updated_at);

CREATE TABLE evaluation_history (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    evaluator_id uuid NOT NULL REFERENCES users (id),
    target_id    uuid NOT NULL REFERENCES users (id),
    value        evaluation_value NOT NULL,
    changed_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX evaluation_history_target_id_idx ON evaluation_history (target_id);
CREATE INDEX evaluation_history_evaluator_id_idx ON evaluation_history (evaluator_id);

CREATE TABLE match (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low   uuid NOT NULL REFERENCES users (id),
    user_high  uuid NOT NULL REFERENCES users (id),
    state      match_state NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (user_low < user_high),
    UNIQUE (user_low, user_high)
);

CREATE INDEX match_user_high_idx ON match (user_high);

CREATE TABLE application (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    candidate_id uuid NOT NULL REFERENCES users (id),
    state        application_state NOT NULL DEFAULT 'pending',
    decided_by   uuid REFERENCES users (id),
    decided_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX application_candidate_id_idx ON application (candidate_id);
CREATE INDEX application_state_idx ON application (state);

CREATE TABLE invite (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token      text NOT NULL UNIQUE,
    created_by uuid NOT NULL REFERENCES users (id),
    used_by    uuid REFERENCES users (id),
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX invite_created_by_idx ON invite (created_by);

CREATE TABLE consent (
    user_id          uuid NOT NULL REFERENCES users (id),
    document_version text NOT NULL,
    granted_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, document_version)
);

-- +goose Down
DROP TABLE consent;
DROP TABLE invite;
DROP TABLE application;
DROP TABLE match;
DROP TABLE evaluation_history;
DROP TABLE evaluation;
DROP TYPE application_state;
DROP TYPE match_state;
DROP TYPE evaluation_source;
DROP TYPE evaluation_value;
