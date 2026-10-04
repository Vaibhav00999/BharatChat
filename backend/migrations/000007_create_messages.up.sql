CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    type message_type NOT NULL DEFAULT 'text',
    body TEXT,
    reply_to_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    forwarded_from_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    forward_count INT NOT NULL DEFAULT 0,
    is_edited BOOLEAN NOT NULL DEFAULT FALSE,
    edited_at TIMESTAMPTZ,
    is_deleted_for_everyone BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at TIMESTAMPTZ,
    client_generated_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_chat_id_created_at ON messages (chat_id, created_at DESC) WHERE is_deleted_for_everyone = FALSE;
CREATE INDEX idx_messages_sender_id ON messages (sender_id);
CREATE INDEX idx_messages_reply_to ON messages (reply_to_message_id);
CREATE UNIQUE INDEX uq_messages_client_generated_id ON messages (chat_id, sender_id, client_generated_id) WHERE client_generated_id IS NOT NULL;
CREATE INDEX idx_messages_body_trgm ON messages USING gin (body gin_trgm_ops) WHERE body IS NOT NULL;
CREATE TRIGGER trg_messages_updated_at BEFORE UPDATE ON messages FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE chats ADD CONSTRAINT fk_chats_last_message FOREIGN KEY (last_message_id) REFERENCES messages(id) ON DELETE SET NULL;
ALTER TABLE chat_participants ADD CONSTRAINT fk_chat_participants_last_read_message FOREIGN KEY (last_read_message_id) REFERENCES messages(id) ON DELETE SET NULL;

CREATE OR REPLACE FUNCTION touch_chat_on_new_message()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE chats SET last_message_id = NEW.id, last_activity_at = NEW.created_at WHERE id = NEW.chat_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_messages_touch_chat AFTER INSERT ON messages FOR EACH ROW EXECUTE FUNCTION touch_chat_on_new_message();
