-- +goose Up
CREATE TYPE moderation_action AS ENUM ('application_approved', 'application_rejected', 'invite_created', 'tag_revoked');
CREATE TYPE moderation_target_type AS ENUM ('application', 'invite', 'tag');
CREATE TYPE notification_type AS ENUM ('match_created', 'application_approved', 'application_rejected', 'tag_revoked', 'last_skill_lost');
CREATE TYPE notification_state AS ENUM ('pending', 'sent', 'blocked', 'cancelled');

CREATE TABLE contact_disclosure_journal (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id                    uuid NOT NULL REFERENCES match (id),
    subject_id                  uuid NOT NULL,
    recipient_id                uuid NOT NULL,
    subject_telegram_user_id    bigint NOT NULL,
    recipient_telegram_user_id  bigint NOT NULL,
    disclosed_at                timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contact_disclosure_journal_match_id_idx ON contact_disclosure_journal (match_id);
CREATE INDEX contact_disclosure_journal_recipient_id_idx ON contact_disclosure_journal (recipient_id);

CREATE TABLE moderation_journal (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    moderator_id uuid NOT NULL REFERENCES users (id),
    action       moderation_action NOT NULL,
    target_type  moderation_target_type NOT NULL,
    target_id    uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX moderation_journal_moderator_id_idx ON moderation_journal (moderator_id);
CREATE INDEX moderation_journal_target_idx ON moderation_journal (target_type, target_id);

CREATE TABLE notification (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type            notification_type NOT NULL,
    user_id         uuid NOT NULL REFERENCES users (id),
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    state           notification_state NOT NULL DEFAULT 'pending',
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notification_user_id_idx ON notification (user_id);
CREATE INDEX notification_due_idx ON notification (next_attempt_at) WHERE state = 'pending';

-- +goose Down
DROP TABLE notification;
DROP TABLE moderation_journal;
DROP TABLE contact_disclosure_journal;
DROP TYPE notification_state;
DROP TYPE notification_type;
DROP TYPE moderation_target_type;
DROP TYPE moderation_action;
