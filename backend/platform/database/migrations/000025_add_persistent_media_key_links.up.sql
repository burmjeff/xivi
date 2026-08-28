-- The keyed hash remains the authentication value. The encrypted token is
-- retained only so an authenticated owner can reconstruct device output URLs.
ALTER TABLE media_access_key ADD COLUMN token_cipher BLOB NULL;
