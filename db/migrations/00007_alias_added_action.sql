-- +goose Up
ALTER TYPE moderation_action ADD VALUE 'alias_added';

-- +goose Down
-- PostgreSQL не поддерживает удаление значения enum; откат не требуется (значение безвредно).
SELECT 1;
