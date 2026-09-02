-- Link ceremonies to the tune that completed them, so the dashboard can
-- show an accurate history (winner + tune) even after later replaces.
ALTER TABLE ceremonies ADD COLUMN tune_id INTEGER REFERENCES tunes(id);
