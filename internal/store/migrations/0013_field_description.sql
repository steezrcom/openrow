-- Optional free-text semantics for a field (allowed values, sign
-- conventions, etc.). Surfaced to the LLM and the UI; never affects DDL.
ALTER TABLE openrow.fields
    ADD COLUMN description TEXT;
