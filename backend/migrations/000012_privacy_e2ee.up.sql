-- Privacy-first account defaults. Existing explicit choices are preserved; these
-- defaults apply to new accounts and reduce metadata exposure before onboarding.
ALTER TABLE users ALTER COLUMN privacy_last_seen SET DEFAULT 'nobody';
ALTER TABLE users ALTER COLUMN privacy_avatar SET DEFAULT 'contacts';
ALTER TABLE users ALTER COLUMN privacy_about SET DEFAULT 'contacts';
ALTER TABLE users ALTER COLUMN privacy_read_receipts SET DEFAULT FALSE;

ALTER TABLE users
    ADD COLUMN privacy_phone VARCHAR(20) NOT NULL DEFAULT 'nobody',
    ADD COLUMN discoverable_by_phone BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN allow_group_adds VARCHAR(20) NOT NULL DEFAULT 'contacts',
    ADD COLUMN share_typing_indicators BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN security_notifications BOOLEAN NOT NULL DEFAULT TRUE,
    ADD CONSTRAINT chk_users_privacy_phone CHECK (privacy_phone IN ('everyone', 'contacts', 'nobody')),
    ADD CONSTRAINT chk_users_allow_group_adds CHECK (allow_group_adds IN ('everyone', 'contacts', 'nobody'));

-- A random app-installation identifier gives device key material and revocation a
-- stable home without collecting hardware serials, advertising IDs, or OS IDs.
ALTER TABLE devices
    ADD COLUMN installation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD CONSTRAINT uq_devices_user_installation UNIQUE (user_id, installation_id),
    ADD CONSTRAINT uq_devices_id_user UNIQUE (id, user_id);

-- Older push-token upserts could reassign a device row while leaving sessions
-- owned by the previous account. Such sessions are unsafe and cannot be repaired.
DELETE FROM sessions s
USING devices d
WHERE s.device_id=d.id AND s.user_id<>d.user_id;

ALTER TABLE sessions
    ADD CONSTRAINT fk_sessions_device_owner
    FOREIGN KEY (device_id, user_id) REFERENCES devices(id, user_id) ON DELETE CASCADE;

-- New chats disappear after seven days by default. Zero explicitly disables
-- expiry; the upper bound prevents an accidental integer-sized retention value.
ALTER TABLE chats
    ADD COLUMN default_message_ttl_seconds INT NOT NULL DEFAULT 604800,
    ADD CONSTRAINT chk_chats_message_ttl
        CHECK (default_message_ttl_seconds BETWEEN 0 AND 31536000);

-- The server stores opaque authenticated ciphertext. body remains nullable for
-- legacy development rows, but production application code rejects new plaintext.
ALTER TABLE messages
    ADD COLUMN ciphertext BYTEA,
	ADD COLUMN encryption_protocol VARCHAR(20),
    ADD COLUMN encryption_version SMALLINT,
    ADD COLUMN sender_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD CONSTRAINT chk_messages_encrypted_content CHECK (
		(ciphertext IS NULL AND encryption_protocol IS NULL AND encryption_version IS NULL)
        OR
        (body IS NULL AND ciphertext IS NOT NULL AND octet_length(ciphertext) >= 16
		 AND encryption_protocol IN ('signal', 'mls')
         AND encryption_version IS NOT NULL AND encryption_version > 0
         AND sender_device_id IS NOT NULL)
    );

CREATE INDEX idx_messages_expires_at ON messages (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_messages_sender_device ON messages (sender_device_id) WHERE sender_device_id IS NOT NULL;

CREATE OR REPLACE FUNCTION set_message_expiry()
RETURNS TRIGGER AS $$
DECLARE
    ttl_seconds INT;
BEGIN
    IF NEW.expires_at IS NULL THEN
        SELECT default_message_ttl_seconds INTO ttl_seconds FROM chats WHERE id = NEW.chat_id;
        IF ttl_seconds > 0 THEN
            NEW.expires_at = NEW.created_at + make_interval(secs => ttl_seconds);
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_messages_set_expiry
BEFORE INSERT ON messages
FOR EACH ROW EXECUTE FUNCTION set_message_expiry();

-- Public key material only. Private identity, signed-prekey, and one-time-prekey
-- secrets never leave the originating device.
CREATE TABLE device_key_bundles (
    device_id                       UUID PRIMARY KEY,
    user_id                         UUID NOT NULL,
    registration_id                 INT NOT NULL CHECK (registration_id BETWEEN 1 AND 16380),
    identity_agreement_public_key   BYTEA NOT NULL CHECK (octet_length(identity_agreement_public_key) = 32),
    identity_signing_public_key     BYTEA NOT NULL CHECK (octet_length(identity_signing_public_key) = 32),
    signed_pre_key_id               INT NOT NULL CHECK (signed_pre_key_id >= 0),
    signed_pre_key_public_key       BYTEA NOT NULL CHECK (octet_length(signed_pre_key_public_key) = 32),
    signed_pre_key_signature        BYTEA NOT NULL CHECK (octet_length(signed_pre_key_signature) = 64),
    key_version                     SMALLINT NOT NULL DEFAULT 1 CHECK (key_version > 0),
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_device_key_bundle_owner
        FOREIGN KEY (device_id, user_id) REFERENCES devices(id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_device_key_bundles_user ON device_key_bundles (user_id);

CREATE TRIGGER trg_device_key_bundles_updated_at
BEFORE UPDATE ON device_key_bundles
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE one_time_pre_keys (
    device_id              UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key_id                 INT NOT NULL CHECK (key_id >= 0),
    public_key             BYTEA NOT NULL CHECK (octet_length(public_key) = 32),
    claimed_at             TIMESTAMPTZ,
    claimed_by_user_id     UUID,
    claimed_by_device_id   UUID,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, key_id),
    CONSTRAINT fk_pre_key_claimant
        FOREIGN KEY (claimed_by_device_id, claimed_by_user_id)
        REFERENCES devices(id, user_id) ON DELETE SET NULL,
    CONSTRAINT chk_pre_key_claim CHECK (
        (claimed_at IS NULL AND claimed_by_user_id IS NULL AND claimed_by_device_id IS NULL)
        OR claimed_at IS NOT NULL
    )
);

CREATE INDEX idx_one_time_pre_keys_available
    ON one_time_pre_keys (device_id, key_id) WHERE claimed_at IS NULL;

-- Retrying bundle discovery from the same authenticated installation returns its
-- original claim instead of consuming the target's entire one-time-key reserve.
CREATE UNIQUE INDEX uq_one_time_pre_key_claim_per_device
    ON one_time_pre_keys (device_id, claimed_by_device_id)
    WHERE claimed_by_device_id IS NOT NULL;
