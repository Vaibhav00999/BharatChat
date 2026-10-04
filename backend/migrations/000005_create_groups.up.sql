CREATE TABLE groups (
    chat_id UUID PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    description VARCHAR(500),
    icon_url TEXT,
    invite_code VARCHAR(32) UNIQUE,
    invite_code_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    only_admins_can_post BOOLEAN NOT NULL DEFAULT FALSE,
    only_admins_can_edit_info BOOLEAN NOT NULL DEFAULT TRUE,
    max_members INT NOT NULL DEFAULT 256,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_groups_invite_code ON groups (invite_code) WHERE invite_code IS NOT NULL;
CREATE TRIGGER trg_groups_updated_at BEFORE UPDATE ON groups FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE FUNCTION check_group_chat_type()
RETURNS TRIGGER AS $$
DECLARE c_type chat_type;
BEGIN
    SELECT type INTO c_type FROM chats WHERE id = NEW.chat_id;
    IF c_type IS DISTINCT FROM 'group' THEN
        RAISE EXCEPTION 'groups.chat_id must reference a chat of type group';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_groups_check_chat_type BEFORE INSERT OR UPDATE ON groups
FOR EACH ROW EXECUTE FUNCTION check_group_chat_type();
