CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number VARCHAR(20) NOT NULL,
    country_code VARCHAR(5) NOT NULL,
    username CITEXT UNIQUE,
    display_name VARCHAR(80) NOT NULL,
    avatar_url TEXT,
    about VARCHAR(200) DEFAULT 'Hey there! I am using BharatChat.',
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen_at TIMESTAMPTZ,
    is_online BOOLEAN NOT NULL DEFAULT FALSE,
    privacy_last_seen VARCHAR(20) NOT NULL DEFAULT 'everyone',
    privacy_avatar VARCHAR(20) NOT NULL DEFAULT 'everyone',
    privacy_about VARCHAR(20) NOT NULL DEFAULT 'everyone',
    privacy_read_receipts BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_users_phone UNIQUE (phone_number)
);

CREATE INDEX idx_users_phone_number ON users (phone_number) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_username_trgm ON users USING gin (username gin_trgm_ops);
CREATE INDEX idx_users_display_name_trgm ON users USING gin (display_name gin_trgm_ops);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
