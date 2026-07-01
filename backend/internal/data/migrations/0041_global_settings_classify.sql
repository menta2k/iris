-- Optional subject-classification feature knobs (off by default). The OpenAI
-- API key is intentionally NOT stored here — it is supplied via the
-- IRIS_OPENAI_API_KEY environment variable, matching the repo's secrets-in-env
-- convention.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS classify_subjects  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS classify_model     TEXT NOT NULL DEFAULT 'gpt-4o-mini',
    ADD COLUMN IF NOT EXISTS classify_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.45,
    ADD COLUMN IF NOT EXISTS classify_api_base  TEXT NOT NULL DEFAULT 'https://api.openai.com/v1';
