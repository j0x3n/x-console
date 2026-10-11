-- +goose Up
-- B160: 稍后阅读可以勾选“下载图片和视频”。勾选的条目抓取时把正文里的视频也存到本机，图片的数量限制放宽。
ALTER TABLE read_items ADD COLUMN download_media INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
