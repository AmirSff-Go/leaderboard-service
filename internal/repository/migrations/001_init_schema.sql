-- Table 1: games (no dependencies)
CREATE TABLE games (
    id UUID PRIMARY KEY,
    name VARCHAR NOT NULL UNIQUE,
    description TEXT,
    token_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table 2: leaderboards (depends on games)
CREATE TYPE leaderboard_type AS ENUM ('Record', 'Additive', 'OneTime');

CREATE TABLE leaderboards (
    id UUID PRIMARY KEY,
    game_id UUID NOT NULL,
    unique_name VARCHAR NOT NULL,
    description TEXT,
    type leaderboard_type NOT NULL,
    interval_seconds INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE,
    UNIQUE(game_id, unique_name)
);

-- Table 3: scores (depends on leaderboards)
CREATE TABLE scores (
    id UUID PRIMARY KEY,
    leaderboard_id UUID NOT NULL,
    user_id VARCHAR NOT NULL,
    score BIGINT NOT NULL,
    duration_index INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (leaderboard_id) REFERENCES leaderboards(id) ON DELETE CASCADE
);

-- Indexes for performance
-- Index for ranking queries (get top scores by leaderboard + duration)
CREATE INDEX idx_scores_leaderboard_duration_score ON scores (leaderboard_id, duration_index, score DESC);

-- Index for user's historical scores
CREATE INDEX idx_scores_leaderboard_user_duration ON scores (leaderboard_id, user_id, duration_index);

-- Index for user's score in a specific period
CREATE INDEX idx_scores_leaderboard_user_duration_score ON scores (leaderboard_id, user_id, duration_index, score);