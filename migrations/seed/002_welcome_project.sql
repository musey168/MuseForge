-- The welcome project for a local installation. It does not require an
-- existing account: when the account named "me" exists it owns the project,
-- otherwise the project is ownerless and the console still shows it.
SET NAMES utf8mb4;
START TRANSACTION;
SET @welcome_owner := (SELECT id FROM users WHERE login_name='me' LIMIT 1);
INSERT IGNORE INTO projects (id,project_key,name,owner_user_id,payload,created_at,updated_at)
VALUES ('welcome','MUSE','MuseForge 初次使用',@welcome_owner,JSON_SET('{"id": "welcome", "key": "MUSE", "name": "MuseForge 初次使用", "description": "写下你的第一条需求，标注状态，做完就把它关掉。", "color": "#3867f4", "createdAt": "2026-10-15T00:00:00Z", "updatedAt": "2026-10-15T00:00:00Z"}','$.ownerId',COALESCE(@welcome_owner,0)),UTC_TIMESTAMP(),UTC_TIMESTAMP());
COMMIT;