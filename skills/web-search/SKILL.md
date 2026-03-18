---
name: web-search
description: "Use when user asks to search the web, research a topic, find information online, or needs current data. Keywords: search, google, look up, find, research, news, what is, how to, latest"
inclusion: on-demand
allowed-tools: "cairn.webSearch,cairn.webFetch"
---

# Web Search

Multi-step web research workflow:

1. **Search** — Use `cairn.webSearch` with a focused query. Start broad, then refine.
2. **Fetch** — Use `cairn.webFetch` on the most relevant URLs from search results.
3. **Summarize** — Extract key facts from fetched content. Note the source URL.
4. **Cite** — Always include source URLs when presenting findings.

## Guidelines

- Start with 3-5 search results, fetch the top 2-3
- If initial results are poor, rephrase the query and search again
- Prefer authoritative sources (official docs, reputable news)
- Summarize concisely — don't dump raw page content
- Always cite sources with URLs
