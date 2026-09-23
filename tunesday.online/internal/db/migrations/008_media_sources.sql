-- Tunes are now labeled by the platform they came from. Existing rows are
-- all YouTube, so the default is correct for them.
ALTER TABLE tunes ADD COLUMN platform TEXT NOT NULL DEFAULT 'youtube';