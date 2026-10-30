-- name: CreateBotLinkNonce :exec
INSERT INTO bot_link_nonce (nonce, telegram_user_id) VALUES ($1, $2);

-- name: ConsumeBotLinkNonce :execrows
UPDATE bot_link_nonce SET used = true
WHERE nonce = $1 AND telegram_user_id = $2 AND NOT used
  AND created_at > now() - make_interval(secs => sqlc.arg(secs));
