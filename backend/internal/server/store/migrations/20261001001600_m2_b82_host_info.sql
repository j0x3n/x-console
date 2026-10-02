-- +goose Up
CREATE TABLE host_info (
    host_id TEXT PRIMARY KEY,
    ownership TEXT NOT NULL DEFAULT 'own' CHECK(ownership IN('own','client')),
    client TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    password_enc TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    sort_order INTEGER NOT NULL DEFAULT 0,
    country_code TEXT NOT NULL DEFAULT '',
    country_checked_at DATETIME,
    country_ip TEXT NOT NULL DEFAULT '',
    addresses TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX host_info_sort ON host_info(sort_order,host_id);
CREATE TABLE host_pairing_info (
    code_hash TEXT PRIMARY KEY REFERENCES pairing_codes(code_hash) ON DELETE CASCADE,
    payload_enc TEXT NOT NULL
);

-- +goose Down
DROP TABLE host_pairing_info;
DROP TABLE host_info;
