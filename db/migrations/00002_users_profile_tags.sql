-- +goose Up
CREATE TYPE user_role AS ENUM ('candidate', 'member', 'moderator');
CREATE TYPE user_access_state AS ENUM ('none', 'invited', 'approved', 'onboarded', 'rejected');
CREATE TYPE meeting_format AS ENUM ('online', 'offline', 'any');
CREATE TYPE tag_status AS ENUM ('active', 'revoked');
CREATE TYPE profile_tag_kind AS ENUM ('skill', 'professional_interest', 'personal_interest');

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_user_id  bigint NOT NULL UNIQUE,
    telegram_username text,
    role              user_role NOT NULL DEFAULT 'candidate',
    access_state      user_access_state NOT NULL DEFAULT 'none',
    created_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz
);

CREATE TABLE profile (
    user_id        uuid PRIMARY KEY REFERENCES users (id),
    first_name     text NOT NULL,
    last_name      text,
    location       text,
    location_norm  text,
    meeting_format meeting_format NOT NULL,
    is_searchable  boolean NOT NULL DEFAULT true,
    fts_doc        tsvector NOT NULL DEFAULT ''::tsvector,
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX profile_fts_doc_gin ON profile USING gin (fts_doc);
CREATE INDEX profile_location_norm_idx ON profile (location_norm);

CREATE TABLE tag (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical  text NOT NULL,
    normalized text NOT NULL UNIQUE,
    status     tag_status NOT NULL DEFAULT 'active',
    created_by uuid NOT NULL REFERENCES users (id)
);

CREATE INDEX tag_normalized_trgm_gin ON tag USING gin (normalized gin_trgm_ops);
CREATE INDEX tag_created_by_idx ON tag (created_by);

CREATE TABLE tag_alias (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tag_id     uuid NOT NULL REFERENCES tag (id),
    alias      text NOT NULL,
    normalized text NOT NULL UNIQUE,
    lexemes    text[] NOT NULL
);

CREATE INDEX tag_alias_tag_id_idx ON tag_alias (tag_id);

CREATE TABLE profile_tag (
    profile_id uuid NOT NULL REFERENCES profile (user_id),
    tag_id     uuid NOT NULL REFERENCES tag (id),
    kind       profile_tag_kind NOT NULL,
    PRIMARY KEY (profile_id, tag_id, kind)
);

CREATE INDEX profile_tag_tag_id_idx ON profile_tag (tag_id);

CREATE TABLE project (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id uuid NOT NULL REFERENCES profile (user_id),
    text       text NOT NULL,
    position   smallint NOT NULL
);

CREATE INDEX project_profile_id_idx ON project (profile_id);

-- +goose Down
DROP TABLE project;
DROP TABLE profile_tag;
DROP TABLE tag_alias;
DROP TABLE tag;
DROP TABLE profile;
DROP TABLE users;
DROP TYPE profile_tag_kind;
DROP TYPE tag_status;
DROP TYPE meeting_format;
DROP TYPE user_access_state;
DROP TYPE user_role;
