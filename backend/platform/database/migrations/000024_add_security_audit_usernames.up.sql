ALTER TABLE security_audit_event ADD COLUMN actor_username VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE security_audit_event ADD COLUMN target_username VARCHAR(64) NOT NULL DEFAULT '';

UPDATE security_audit_event
SET actor_username = COALESCE(
        (SELECT username FROM app_user WHERE id = security_audit_event.actor_user_id),
        ''
    ),
    target_username = COALESCE(
        (SELECT username FROM app_user WHERE id = security_audit_event.target_user_id),
        ''
    );
