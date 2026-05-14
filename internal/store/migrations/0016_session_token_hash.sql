-- Session tokens are no longer stored in plaintext. The application now
-- hashes the cookie value with SHA-256 before writing or comparing the
-- sessions.id column. Existing rows can't be migrated (we can't reverse
-- the hash to find the original token), so they're dropped. Users will
-- need to log in again once; no harm done.
DELETE FROM openrow.sessions;
