CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "citext";

CREATE TYPE chat_type AS ENUM ('direct', 'group');
CREATE TYPE member_role AS ENUM ('owner', 'admin', 'member');
CREATE TYPE message_type AS ENUM ('text', 'image', 'video', 'document', 'audio', 'voice_note', 'system');
CREATE TYPE message_delivery_state AS ENUM ('sent', 'delivered', 'read');
CREATE TYPE device_platform AS ENUM ('android', 'ios', 'web', 'desktop');
CREATE TYPE notification_type AS ENUM ('message', 'group_invite', 'reaction', 'mention', 'call', 'system');
CREATE TYPE report_reason AS ENUM ('spam', 'harassment', 'nudity', 'violence', 'fraud', 'other');
