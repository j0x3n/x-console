-- +goose Up
-- 联系人导入和 iCloud 同步：电话和邮箱是 JSON 数组。source 空是手动建的，vcard 是导入文件，icloud 是同步。
-- external_id 是 vCard 的 UID，重复导入或同步时用它对上已有的联系人。
ALTER TABLE contacts ADD COLUMN phones TEXT NOT NULL DEFAULT '[]';
ALTER TABLE contacts ADD COLUMN emails TEXT NOT NULL DEFAULT '[]';
ALTER TABLE contacts ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE INDEX contacts_external ON contacts(external_id) WHERE external_id <> '';

-- +goose Down
SELECT 1;
