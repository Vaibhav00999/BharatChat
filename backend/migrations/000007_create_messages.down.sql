DROP TRIGGER IF EXISTS trg_messages_touch_chat ON messages;
DROP FUNCTION IF EXISTS touch_chat_on_new_message();
ALTER TABLE chat_participants DROP CONSTRAINT IF EXISTS fk_chat_participants_last_read_message;
ALTER TABLE chats DROP CONSTRAINT IF EXISTS fk_chats_last_message;
DROP TRIGGER IF EXISTS trg_messages_updated_at ON messages;
DROP TABLE IF EXISTS messages;
