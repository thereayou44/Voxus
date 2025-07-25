-- Включаем расширение для UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Создаем таблицу пользователей
CREATE TABLE IF NOT EXISTS users (
                                     id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                     username VARCHAR(50) UNIQUE NOT NULL,
                                     email VARCHAR(255) UNIQUE NOT NULL,
                                     password_hash VARCHAR(255) NOT NULL,
                                     avatar_url VARCHAR(500),
                                     last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для users
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_last_seen ON users(last_seen_at);

-- Создаем таблицу комнат
CREATE TABLE IF NOT EXISTS rooms (
                                     id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                     name VARCHAR(100) NOT NULL,
                                     type VARCHAR(20) NOT NULL CHECK (type IN ('direct', 'group')),
                                     max_members INT DEFAULT 20,
                                     created_by UUID NOT NULL,
                                     created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                     CONSTRAINT fk_rooms_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
);

-- Индексы для rooms
CREATE INDEX idx_rooms_type ON rooms(type);
CREATE INDEX idx_rooms_created_by ON rooms(created_by);

-- Создаем промежуточную таблицу для связи пользователей и комнат
CREATE TABLE IF NOT EXISTS room_members (
                                            user_id UUID NOT NULL,
                                            room_id UUID NOT NULL,
                                            joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                            role VARCHAR(20) DEFAULT 'member' CHECK (role IN ('member', 'moderator', 'admin')),
                                            last_read_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                            PRIMARY KEY (user_id, room_id),
                                            CONSTRAINT fk_room_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
                                            CONSTRAINT fk_room_members_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);

-- Индексы для room_members
CREATE INDEX idx_room_members_room_id ON room_members(room_id);
CREATE INDEX idx_room_members_user_id ON room_members(user_id);
CREATE INDEX idx_room_members_joined_at ON room_members(joined_at);

-- Создаем таблицу сообщений
CREATE TABLE IF NOT EXISTS messages (
                                        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                        room_id UUID NOT NULL,
                                        user_id UUID NOT NULL,
                                        content TEXT NOT NULL,
                                        type VARCHAR(20) DEFAULT 'text' CHECK (type IN ('text', 'image', 'file', 'system')),
                                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                        edited_at TIMESTAMP,
                                        deleted_at TIMESTAMP,
                                        CONSTRAINT fk_messages_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
                                        CONSTRAINT fk_messages_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Индексы для messages
CREATE INDEX idx_messages_room_id ON messages(room_id);
CREATE INDEX idx_messages_user_id ON messages(user_id);
CREATE INDEX idx_messages_created_at ON messages(created_at DESC);
CREATE INDEX idx_messages_room_created ON messages(room_id, created_at DESC);

-- Создаем таблицу для вложений
CREATE TABLE IF NOT EXISTS message_attachments (
                                                   id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                                   message_id UUID NOT NULL,
                                                   file_name VARCHAR(255) NOT NULL,
                                                   file_size BIGINT NOT NULL,
                                                   file_type VARCHAR(100),
                                                   file_url VARCHAR(500) NOT NULL,
                                                   thumbnail_url VARCHAR(500),
                                                   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                                   CONSTRAINT fk_attachments_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

-- Индекс для attachments
CREATE INDEX idx_attachments_message_id ON message_attachments(message_id);

-- Создаем таблицу для push токенов (для будущих уведомлений)
CREATE TABLE IF NOT EXISTS push_tokens (
                                           id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                           user_id UUID NOT NULL,
                                           token VARCHAR(500) NOT NULL,
                                           platform VARCHAR(20) NOT NULL CHECK (platform IN ('web', 'ios', 'android')),
                                           created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                           updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                           CONSTRAINT fk_push_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
                                           UNIQUE(user_id, token)
);

-- Индексы для push_tokens
CREATE INDEX idx_push_tokens_user_id ON push_tokens(user_id);

-- Создаем таблицу для блокировок пользователей
CREATE TABLE IF NOT EXISTS user_blocks (
                                           blocker_id UUID NOT NULL,
                                           blocked_id UUID NOT NULL,
                                           created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                           PRIMARY KEY (blocker_id, blocked_id),
                                           CONSTRAINT fk_blocks_blocker FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
                                           CONSTRAINT fk_blocks_blocked FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE,
                                           CONSTRAINT check_not_self_block CHECK (blocker_id != blocked_id)
);

-- Индексы для user_blocks
CREATE INDEX idx_user_blocks_blocker ON user_blocks(blocker_id);
CREATE INDEX idx_user_blocks_blocked ON user_blocks(blocked_id);

-- Создаем таблицу для реакций на сообщения (для будущего функционала)
CREATE TABLE IF NOT EXISTS message_reactions (
                                                 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                                 message_id UUID NOT NULL,
                                                 user_id UUID NOT NULL,
                                                 emoji VARCHAR(10) NOT NULL,
                                                 created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                                                 CONSTRAINT fk_reactions_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
                                                 CONSTRAINT fk_reactions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
                                                 UNIQUE(message_id, user_id, emoji)
);

-- Индексы для reactions
CREATE INDEX idx_reactions_message_id ON message_reactions(message_id);

-- Функция для автоматического обновления last_seen_at
CREATE OR REPLACE FUNCTION update_user_last_seen()
    RETURNS TRIGGER AS $$
BEGIN
    UPDATE users
    SET last_seen_at = CURRENT_TIMESTAMP
    WHERE id = NEW.user_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Триггер для обновления last_seen при отправке сообщения
CREATE TRIGGER trigger_update_last_seen
    AFTER INSERT ON messages
    FOR EACH ROW
EXECUTE FUNCTION update_user_last_seen();

-- Функция для проверки лимита участников комнаты
CREATE OR REPLACE FUNCTION check_room_member_limit()
    RETURNS TRIGGER AS $$
DECLARE
    current_count INT;
    max_count INT;
BEGIN
    -- Получаем текущее количество участников и максимум
    SELECT COUNT(*), r.max_members INTO current_count, max_count
    FROM room_members rm
             JOIN rooms r ON r.id = rm.room_id
    WHERE rm.room_id = NEW.room_id
    GROUP BY r.max_members;

    -- Проверяем лимит
    IF current_count >= max_count THEN
        RAISE EXCEPTION 'Room member limit exceeded';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Триггер для проверки лимита участников
CREATE TRIGGER trigger_check_room_limit
    BEFORE INSERT ON room_members
    FOR EACH ROW
EXECUTE FUNCTION check_room_member_limit();

-- Представление для получения последних сообщений в комнатах
CREATE OR REPLACE VIEW room_last_messages AS
SELECT DISTINCT ON (m.room_id)
    m.room_id,
    m.id as message_id,
    m.content,
    m.user_id,
    m.created_at,
    u.username,
    u.avatar_url
FROM messages m
         JOIN users u ON u.id = m.user_id
WHERE m.deleted_at IS NULL
ORDER BY m.room_id, m.created_at DESC;

-- Представление для подсчета непрочитанных сообщений
CREATE OR REPLACE VIEW unread_counts AS
SELECT
    rm.user_id,
    rm.room_id,
    COUNT(m.id) as unread_count
FROM room_members rm
         LEFT JOIN messages m ON m.room_id = rm.room_id
    AND m.created_at > rm.last_read_at
    AND m.user_id != rm.user_id
    AND m.deleted_at IS NULL
GROUP BY rm.user_id, rm.room_id;

-- Добавляем некоторые полезные функции

-- Функция для создания direct комнаты между двумя пользователями
CREATE OR REPLACE FUNCTION create_or_get_direct_room(user1_id UUID, user2_id UUID)
    RETURNS UUID AS $$
DECLARE
    room_id UUID;
BEGIN
    -- Проверяем, существует ли уже direct комната между этими пользователями
    SELECT r.id INTO room_id
    FROM rooms r
    WHERE r.type = 'direct'
      AND EXISTS (
        SELECT 1 FROM room_members rm1
        WHERE rm1.room_id = r.id AND rm1.user_id = user1_id
    )
      AND EXISTS (
        SELECT 1 FROM room_members rm2
        WHERE rm2.room_id = r.id AND rm2.user_id = user2_id
    )
    LIMIT 1;

    -- Если комната найдена, возвращаем её ID
    IF room_id IS NOT NULL THEN
        RETURN room_id;
    END IF;

    -- Создаем новую direct комнату
    INSERT INTO rooms (name, type, max_members, created_by)
    VALUES ('Direct', 'direct', 2, user1_id)
    RETURNING id INTO room_id;

    -- Добавляем обоих пользователей
    INSERT INTO room_members (user_id, room_id) VALUES
                                                    (user1_id, room_id),
                                                    (user2_id, room_id);

    RETURN room_id;
END;
$$ LANGUAGE plpgsql;

-- Гранты для пользователя приложения
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO postgres;