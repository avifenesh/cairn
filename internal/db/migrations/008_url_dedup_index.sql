-- Add URL index for cross-source deduplication.
-- Feed items from dev.to may arrive via both the devto poller and the rss poller
-- (since dev.to feeds are configured in RSS_FEEDS). This index enables efficient
-- URL-based dedup across sources.
CREATE INDEX IF NOT EXISTS idx_events_url ON events(url);
