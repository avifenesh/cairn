# Summarize Skill -- Reference

Extended reference for content extraction tools and advanced patterns.

## Tool Installation

The summarize skill requires CLI tools for content extraction. Install as needed:

```bash
# PDF extraction (poppler-utils)
sudo apt install -y poppler-utils
# Verify: pdftotext -v

# YouTube subtitle extraction
pip3 install yt-dlp
# Verify: yt-dlp --version

# Audio/video processing (for format conversion before transcription)
sudo apt install -y ffmpeg
# Verify: ffmpeg -version

# OCR for images
sudo apt install -y tesseract-ocr
# Verify: tesseract --version
```

All tools are optional -- the skill gracefully suggests installation when a tool is missing.

## Advanced YouTube Patterns

### Subtitle Language Selection

```bash
# List available subtitles
yt-dlp --list-subs 'VIDEO_URL' 2>/dev/null

# Extract specific language (Hebrew)
yt-dlp --write-auto-sub --skip-download --sub-lang he -o "$TMPDIR/%(id)s" 'VIDEO_URL'

# Prefer manual subs over auto-generated
yt-dlp --write-sub --write-auto-sub --skip-download --sub-lang en -o "$TMPDIR/%(id)s" 'VIDEO_URL'
```

### Cleaning VTT Subtitle Files

VTT files contain timestamps and duplicate lines. Clean extraction:

```bash
cat "$TMPDIR"/*.vtt | \
  grep -vE '^(WEBVTT|Kind:|Language:|$)' | \
  grep -vF -- '-->' | \
  grep -vE '^\d{2}:\d{2}' | \
  awk '!seen[$0]++' | \
  head -500
```

### Video Metadata

Get title and description without downloading:

```bash
yt-dlp --print title --print description --skip-download 'VIDEO_URL' 2>/dev/null | head -20
```

## AWS Transcribe Workflow

For audio files, AWS Transcribe provides high-quality transcription. This is an async process.

### Prerequisites

- AWS CLI configured with `--region eu-central-1`
- An S3 bucket for temporary audio storage
- IAM permissions for `transcribe:*` and `s3:PutObject/GetObject`

### Step-by-Step

```bash
# 1. Convert to supported format if needed (mp3, wav, flac, ogg)
ffmpeg -i input.m4a -ar 16000 -ac 1 /tmp/audio.wav

# 2. Upload to S3
BUCKET="pub-transcribe-temp"
UID=$(cat /proc/sys/kernel/random/uuid)
KEY="audio/${UID}.wav"
aws s3 cp /tmp/audio.wav "s3://$BUCKET/$KEY" --region eu-central-1 --sse AES256

# 3. Start transcription job
JOB_NAME="pub-${UID}"
aws transcribe start-transcription-job \
  --transcription-job-name "$JOB_NAME" \
  --language-code en-US \
  --media "MediaFileUri=s3://$BUCKET/$KEY" \
  --region eu-central-1

# 4. Poll for completion (check every 30s, max 10 min)
for i in $(seq 1 20); do
  STATUS=$(aws transcribe get-transcription-job \
    --transcription-job-name "$JOB_NAME" \
    --region eu-central-1 \
    --query 'TranscriptionJob.TranscriptionJobStatus' \
    --output text)
  [ "$STATUS" = "COMPLETED" ] && break
  [ "$STATUS" = "FAILED" ] && { echo "Transcription failed"; exit 1; }
  sleep 30
done

# 5. Download transcript
TRANSCRIPT_URI=$(aws transcribe get-transcription-job \
  --transcription-job-name "$JOB_NAME" \
  --region eu-central-1 \
  --query 'TranscriptionJob.Transcript.TranscriptFileUri' \
  --output text)
curl -s "$TRANSCRIPT_URI" | jq -r '.results.transcripts[0].transcript'

# 6. Cleanup
aws s3 rm "s3://$BUCKET/$KEY" --region eu-central-1
aws transcribe delete-transcription-job \
  --transcription-job-name "$JOB_NAME" \
  --region eu-central-1
```

### Language Detection

AWS Transcribe can auto-detect language:

```bash
aws transcribe start-transcription-job \
  --transcription-job-name "$JOB_NAME" \
  --identify-language \
  --language-options en-US,he-IL,de-DE \
  --media "MediaFileUri=s3://$BUCKET/$KEY" \
  --region eu-central-1
```

## Content Size Handling

### Chunked Summarization for Large Content

When extracted text exceeds ~50KB (approximately 500 lines):

1. Split into chunks of 400 lines each
2. Summarize each chunk independently (bullet format)
3. Combine chunk summaries into a final summary
4. The final summary should be shorter than the sum of chunk summaries

```bash
# Split large file into chunks
split -l 400 extracted_text.txt /tmp/chunk_

# Process each chunk (the assistant reads each via cairn.shell)
for chunk in /tmp/chunk_*; do
  cat "$chunk"
  # Assistant summarizes this chunk
done
```

### PDF Page Ranges

For large PDFs, extract specific sections:

```bash
# First 20 pages
pdftotext -f 1 -l 20 -layout document.pdf -

# Pages 50-70
pdftotext -f 50 -l 70 -layout document.pdf -

# Just the first and last pages (intro + conclusion)
LAST=$(pdfinfo document.pdf | grep Pages | awk '{print $2}') && \
pdftotext -f 1 -l 3 -layout document.pdf - && \
if [ "$LAST" -gt 3 ]; then echo "---" && \
  pdftotext -f $((LAST-2)) -l $LAST -layout document.pdf -; fi
```

## Alternative Extraction for Blocked Sites

Some sites block curl with CAPTCHAs or JavaScript rendering requirements.

**Options:**
1. Ask the user to copy-paste the article text directly
2. Use the `/agent-browser` skill to render the page in a headless browser
3. Try Google's cache: `curl -sL "https://webcache.googleusercontent.com/search?q=cache:URL"`
4. Try the Wayback Machine: `curl -sL "https://web.archive.org/web/URL"`
