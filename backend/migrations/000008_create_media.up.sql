CREATE TABLE media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    uploader_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL,
    storage_bucket VARCHAR(120) NOT NULL,
    mime_type VARCHAR(120) NOT NULL,
    file_name VARCHAR(255),
    file_size_bytes BIGINT NOT NULL,
    width INT,
    height INT,
    duration_ms INT,
    thumbnail_storage_key TEXT,
    checksum_sha256 VARCHAR(64),
    is_voice_note BOOLEAN NOT NULL DEFAULT FALSE,
    upload_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_media_message_id ON media (message_id);
CREATE INDEX idx_media_uploader_id ON media (uploader_id);

CREATE TABLE media_purposes (
    media_id UUID PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
    purpose VARCHAR(30) NOT NULL,
    owner_ref_id UUID NOT NULL
);
CREATE INDEX idx_media_purposes_owner ON media_purposes (owner_ref_id, purpose);
