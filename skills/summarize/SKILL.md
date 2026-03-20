---
name: summarize
description: "Universal content summarization. Use when asked to summarize a URL, article, PDF, YouTube video, audio file, or any text content. Supports configurable output formats. Keywords: summarize, tldr, summary, digest, recap, extract, transcribe, what is this about, explain this link, key takeaways, bullet points"
argument-hint: "<URL, file path, or paste content to summarize>"
allowed-tools: "cairn.shell,cairn.createArtifact"
inclusion: on-demand
---

# Universal Summarizer

Summarize any content: URLs, PDFs, YouTube videos, audio files, or pasted text. The assistant extracts content via shell tools, then summarizes it directly.

Adapted from OpenClaw `summarize` concept. See `reference.md` for tool installation and advanced patterns.

## Step 1: Detect Content Type

Determine the content type from `$ARGUMENTS` or conversation context:

| Input Pattern | Type | Extraction Method |
|--------------|------|-------------------|
| `https://youtube.com/...` or `https://youtu.be/...` | YouTube | yt-dlp subtitle extraction |
| `https://...` or `http://...` | Web page | curl + HTML stripping |
| Path ending in `.pdf` | PDF | pdftotext |
| Path ending in `.mp3`, `.wav`, `.m4a`, `.flac`, `.ogg` | Audio | AWS Transcribe |
| Path ending in `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp` | Image | Describe directly (multimodal) |
| Everything else | Plain text | Use directly |

## Step 2: Extract Content

**Security**: Before fetching any URL, verify it does NOT target: private IPs (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16), loopback (127.0.0.0/8 except 127.0.0.1:8888 for SearXNG), link-local (169.254.0.0/16), cloud metadata (169.254.169.254), IPv6 private (::1, fc00::/7, fe80::/10), or non-HTTP(S) protocols (file://, ftp://).

**Shell safety**: All placeholders below (URL_HERE, FILE_PATH, VIDEO_URL) must be properly quoted. Insert actual values inside single quotes. If a value contains single quotes, escape them with `'\''` (end quote, escaped quote, start quote). Never use unescaped `$()` or backticks in user-provided values.

### Web Page

Use `cairn.shell` to fetch and extract text:

```bash
curl -sL --max-time 15 --max-filesize 1048576 -A 'Mozilla/5.0' 'URL_HERE' | \
  sed 's/<script[^>]*>.*<\/script>//g; s/<style[^>]*>.*<\/style>//g; s/<[^>]*>//g' | \
  sed '/^[[:space:]]*$/d' | head -500
```

If the output is mostly empty or a login wall, tell the user and suggest they copy-paste the article text.

### PDF

Requires `pdftotext` from poppler-utils. If not installed, tell the user:
```
sudo apt install poppler-utils
```

Extract text:
```bash
pdftotext -layout 'FILE_PATH' - | head -1000
```

For large PDFs, extract specific pages:
```bash
pdftotext -f 1 -l 20 -layout 'FILE_PATH' - | head -1000
```

### YouTube

Requires `yt-dlp`. If not installed, tell the user:
```
pip3 install yt-dlp
```

Extract auto-generated subtitles:
```bash
TMPDIR=$(mktemp -d) && \
yt-dlp --write-auto-sub --skip-download --sub-lang 'en.*' \
  -o "$TMPDIR/%(id)s" 'VIDEO_URL' 2>/dev/null && \
cat "$TMPDIR"/*.vtt 2>/dev/null | \
  grep -vE '^(WEBVTT|Kind:|Language:|\d{2}:\d{2}|-->|$)' | \
  awk '!seen[$0]++' | head -500 && \
rm -rf "$TMPDIR"
```

If no subtitles are available, tell the user. Suggest they try a different subtitle language (replace `en.*` with `es`, `de`, etc.) or that this video has no captions.

### Audio

Audio transcription via AWS Transcribe is a multi-step async process. See `reference.md` for the full workflow. For quick transcription of short clips, suggest:

1. Upload to S3: `aws s3 cp FILE s3://BUCKET/audio/ --region eu-central-1`
2. Start transcription job with AWS CLI
3. Poll for completion
4. Download and extract transcript text

If the user just wants a quick summary of a podcast, suggest they provide a URL to the show notes page instead.

### Image

The assistant is multimodal -- it can describe images shown directly in conversation. For image files on disk, if `tesseract` is installed, extract text via OCR:

```bash
tesseract 'IMAGE_PATH' stdout 2>/dev/null | head -200
```

If tesseract is not installed: `sudo apt install tesseract-ocr`

Otherwise, ask the user to paste/show the image directly in conversation.

### Plain Text

Use the content directly -- no extraction needed. If the user pasted text, summarize it as-is. If they provided a file path (not PDF/audio/image), read it first:

```bash
cat 'FILE_PATH' | head -1000
```

## Step 3: Choose Output Format

Ask the user or infer from their request which format they want:

| Format | Trigger Words | Output |
|--------|--------------|--------|
| **bullet** (default) | "summarize", "key points", "tldr" | 5-10 bullet points, most important first |
| **paragraph** | "one paragraph", "brief", "short" | 3-5 sentence paragraph |
| **detailed** | "detailed", "comprehensive", "full" | Multi-section: Key Takeaways + Main Points + Details + Conclusion |
| **takeaways** | "takeaways", "actionable", "what matters" | 3-5 actionable insights only |

If unclear, default to **bullet** format.

## Step 4: Summarize

Read the extracted content and produce the summary. Follow these rules:

- **Accuracy first**: Never invent claims, statistics, or quotes not in the source
- **Attribution**: Always note the source (URL, filename, video title) at the top
- **Key information**: Preserve names, dates, numbers, and specific claims
- **Structure**: For long content, cover introduction and conclusion -- the middle often has supporting detail
- **Length**: Match the format -- bullets are concise, detailed is thorough
- **Objectivity**: Present the content's perspective, not your own opinion

For content longer than what fits in context, use chunked extraction:
1. First chunk: use `head -500` to get the beginning
2. Next chunk: use `sed -n '501,1000p'` to get lines 501-1000
3. Summarize each chunk, then combine chunk summaries into a final summary

## Step 5: Save as Artifact

After presenting the summary inline, save it as a persistent artifact:

```js
cairn.createArtifact({
  "type": "summary",
  "title": "Summary of [source title or URL]",
  "contentJson": {
    "sections": [
      { "heading": "Source", "text": "[URL or file path]" },
      { "heading": "Key Takeaways", "text": "- Point 1\n- Point 2\n- Point 3" },
      { "heading": "Summary", "text": "The detailed summary content..." }
    ]
  }
})
```

Two content formats are supported by `renderSummary()`: simple `{ "text": "full summary" }` for quick summaries, or `{ "sections": [...] }` for structured output. Prefer the sectioned format for detailed summaries.

**Always save as artifact** so the summary is retrievable later ("what was that article I summarized last week?").

## Limitations

- **Paywalled content**: curl gets login walls. Ask the user to copy-paste the text
- **No subtitles on YouTube**: Some videos lack auto-generated captions. No workaround without audio transcription
- **Large files**: Shell output capped at 100KB. Very long PDFs are truncated. Summarize in chunks if needed
- **Audio transcription**: Requires AWS Transcribe (async, multi-step). Not instant. See `reference.md`
- **Rate limits**: Fetching URLs counts against normal shell execution. Don't batch-fetch dozens of URLs

## Notes

- SSRF protection rules are inlined in Step 2 above
- For research across multiple URLs, use the `/research` skill instead
- For summarizing Cairn's own feed items, use the `/digest` skill instead
- Tool installation commands are in `reference.md`
