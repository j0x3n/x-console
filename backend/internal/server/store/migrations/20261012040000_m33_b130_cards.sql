-- +goose Up
-- B130: review cards. FSRS state lives on the card, every rating is logged.
CREATE TABLE cards (
    id INTEGER PRIMARY KEY,
    deck TEXT NOT NULL DEFAULT '默认',
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    source_title TEXT NOT NULL DEFAULT '',
    source_link TEXT NOT NULL DEFAULT '',
    suspended INTEGER NOT NULL DEFAULT 0,
    state INTEGER NOT NULL DEFAULT 0,
    stability REAL NOT NULL DEFAULT 0,
    difficulty REAL NOT NULL DEFAULT 0,
    due DATETIME NOT NULL,
    last_review DATETIME,
    scheduled_days INTEGER NOT NULL DEFAULT 0,
    reps INTEGER NOT NULL DEFAULT 0,
    lapses INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX cards_queue ON cards(suspended, state, due);
CREATE INDEX cards_deck ON cards(deck);

CREATE TABLE card_reviews (
    id INTEGER PRIMARY KEY,
    card_id INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    rating INTEGER NOT NULL,
    state_before INTEGER NOT NULL,
    reviewed_at DATETIME NOT NULL,
    scheduled_days INTEGER NOT NULL DEFAULT 0,
    stability REAL NOT NULL DEFAULT 0,
    difficulty REAL NOT NULL DEFAULT 0
);
CREATE INDEX card_reviews_at ON card_reviews(reviewed_at);
CREATE INDEX card_reviews_card ON card_reviews(card_id);

-- +goose Down
DROP TABLE card_reviews;
DROP TABLE cards;
