ALTER TABLE openrow.llm_configs
    ADD COLUMN last_tested_at      TIMESTAMPTZ,
    ADD COLUMN last_test_ok        BOOLEAN,
    ADD COLUMN last_test_tools_ok  BOOLEAN,
    ADD COLUMN last_test_message   TEXT;
