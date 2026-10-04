DROP TABLE IF EXISTS one_time_pre_keys;
DROP TRIGGER IF EXISTS trg_device_key_bundles_updated_at ON device_key_bundles;
DROP TABLE IF EXISTS device_key_bundles;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS fk_sessions_device_owner;
ALTER TABLE devices DROP CONSTRAINT IF EXISTS uq_devices_id_user;
ALTER TABLE devices DROP CONSTRAINT IF EXISTS uq_devices_user_installation;
ALTER TABLE devices DROP COLUMN IF EXISTS installation_id;

DROP TRIGGER IF EXISTS trg_messages_set_expiry ON messages;
DROP FUNCTION IF EXISTS set_message_expiry();
DROP INDEX IF EXISTS idx_messages_sender_device;
DROP INDEX IF EXISTS idx_messages_expires_at;
ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS chk_messages_encrypted_content,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS sender_device_id,
    DROP COLUMN IF EXISTS encryption_version,
	DROP COLUMN IF EXISTS encryption_protocol,
    DROP COLUMN IF EXISTS ciphertext;

ALTER TABLE chats
    DROP CONSTRAINT IF EXISTS chk_chats_message_ttl,
    DROP COLUMN IF EXISTS default_message_ttl_seconds;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS chk_users_allow_group_adds,
    DROP CONSTRAINT IF EXISTS chk_users_privacy_phone,
    DROP COLUMN IF EXISTS security_notifications,
    DROP COLUMN IF EXISTS share_typing_indicators,
    DROP COLUMN IF EXISTS allow_group_adds,
    DROP COLUMN IF EXISTS discoverable_by_phone,
    DROP COLUMN IF EXISTS privacy_phone;

ALTER TABLE users ALTER COLUMN privacy_read_receipts SET DEFAULT TRUE;
ALTER TABLE users ALTER COLUMN privacy_about SET DEFAULT 'everyone';
ALTER TABLE users ALTER COLUMN privacy_avatar SET DEFAULT 'everyone';
ALTER TABLE users ALTER COLUMN privacy_last_seen SET DEFAULT 'everyone';
