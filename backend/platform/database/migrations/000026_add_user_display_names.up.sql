ALTER TABLE app_user ADD COLUMN display_name VARCHAR(80) NOT NULL DEFAULT '';

-- Audit identity is snapshotted so later renames never rewrite history.
ALTER TABLE security_audit_event ADD COLUMN actor_display_name VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE security_audit_event ADD COLUMN target_display_name VARCHAR(80) NOT NULL DEFAULT '';

UPDATE security_audit_event
SET actor_display_name = COALESCE(
        (SELECT display_name FROM app_user WHERE id = security_audit_event.actor_user_id),
        ''
    ),
    target_display_name = COALESCE(
        (SELECT display_name FROM app_user WHERE id = security_audit_event.target_user_id),
        ''
    );
