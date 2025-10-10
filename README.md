# dontshowmethis

An AI-powered Bluesky content moderation system that automatically labels replies to monitored accounts using LLM-based content analysis.

## Overview

This project monitors Bluesky posts in real-time via Jetstream and uses a local LLM (via LM Studio) to classify replies to watched accounts. It automatically applies labels such as "bad-faith", "off-topic", and "funny" to help users filter and moderate content.

The system consists of two components:
1. **Go Consumer** - Monitors the Jetstream firehose and analyzes replies using an LLM
2. **Skyware Labeler** - TypeScript server that manages and emits content labels

## Architecture

```
Jetstream → Go Consumer → LM Studio (LLM) → Labeler Service → Bluesky
```

1. The Go consumer subscribes to Jetstream and monitors replies to specified accounts
2. When a reply is detected, it fetches the parent post and sends both to LM Studio
3. The LLM classifies the reply based on the system prompt
4. Labels are emitted via the Skyware labeler service
5. Labels are propagated to Bluesky's labeling system

## Prerequisites

- [LM Studio](https://lmstudio.ai/) with a compatible model loaded
- A Bluesky account for the labeler

## Installation

### Clone the repository

```bash
git clone https://github.com/haileyok/dontshowmethis.git
cd dontshowmethis
```

### Install Go dependencies

```bash
go mod download
```

### Install labeler dependencies

```bash
cd labeler
yarn install
cd ..
```

## Configuration

Copy the example environment file and configure it:

```bash
cp .env.example .env
```

### Environment Variables

**For the Go Consumer:**

- `PDS_URL` - Your Bluesky PDS URL (e.g., `https://bsky.social`)
- `ACCOUNT_HANDLE` - Your Bluesky account handle
- `ACCOUNT_PASSWORD` - Your Bluesky account password
- `WATCHED_OPS` - Comma-separated list of DIDs to monitor for replies and emit labels for
- `WATCHED_LOG_OPS` - Comma-separated list of DIDs to monitor for replies but not emit labels for. Will use SQLite to keep a log
- `LOGGED_LABELS` - Comma-separated list of labels that will be logged to the SQLite database
- `JETSTREAM_URL` - Jetstream WebSocket URL (default: `wss://jetstream2.us-west.bsky.network/subscribe`)
- `LABELER_URL` - URL of your labeler service (e.g., `http://localhost:3000`)
- `LABELER_KEY` - Authentication key for the labeler API
- `LMSTUDIO_HOST` - LM Studio API host (e.g., `http://localhost:1234`)
- `LOG_DB_NAME` - The name of the SQLite db to log to

**For the Skyware Labeler:**

- `SKYWARE_DID` - Your labeler's DID
- `SKYWARE_SIG_KEY` - Your labeler's signing key
- `EMIT_LABEL_KEY` - Secret key for the emit label API (must match `LABELER_KEY` above)

## Running the Services

### 1. Start LM Studio

1. Open LM Studio
2. Load a compatible model (recommended: `google/gemma-3-27b` or similar)
3. Start the local server (usually runs on `http://localhost:1234`)

### 2. Start the Labeler Service

```bash
cd labeler
npm start
```

The labeler will start two servers:
- Port 14831: Skyware labeler server
- Port 3000: Label emission API

### 3. Start the Go Consumer

```bash
go run .
```

Or build and run:

```bash
go build -o dontshowmethis
./dontshowmethis
```

## Usage

Once running, the system will:

1. Connect to Jetstream and monitor the firehose
2. Watch for replies to accounts specified in `WATCHED_OPS`
3. Automatically analyze and label qualifying replies
4. Log all actions to stdout

### Finding Account DIDs

To monitor specific accounts, you need their DIDs. You can find a DID by:

```bash
curl "https://bsky.social/xrpc/com.atproto.identity.resolveHandle?handle=username.bsky.social"
```

Add the returned DID to your `WATCHED_OPS` environment variable.

## How Content Classification Works

See `lmstudio.go:147` for the system prompt.

## Development

### Project Structure

```
.
├── main.go              # CLI setup and consumer initialization
├── handle_post.go       # Post handling and labeling logic
├── lmstudio.go         # LLM client and content classification
├── sets/
│   └── domains.go      # Political domain list (currently unused)
├── labeler/
│   ├── index.ts        # Skyware labeler service
│   └── package.json    # Labeler dependencies
├── .env.example        # Example environment configuration
└── README.md           # This file
```

### Adding New Labels

1. Add the label constant in `main.go`:
   ```go
   const LabelNewLabel = "new-label"
   ```

2. Add it to the labeler's allowed labels in `labeler/index.ts`:
   ```typescript
   const LABELS: Record<string, boolean> = {
     'bad-faith': true,
     'off-topic': true,
     'funny': true,
     'new-label': true,  // Add here
   }
   ```

3. Update the LLM schema in `lmstudio.go` to include the new classification

4. Update the handling logic in `handle_post.go` to emit the new label

## License

MIT

## Acknowledgments

- [Jetstream](https://github.com/bluesky-social/jetstream) - Real-time firehose for Bluesky
- [Skyware](https://skyware.js.org/) - Labeler framework
- [LM Studio](https://lmstudio.ai/) - Local LLM inference
