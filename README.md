# AWS Cursor Router (Go)

High-performance OpenAI-compatible proxy for Cursor + AWS Bedrock.

AWS credentials stay on the server. Team members only use proxy API keys.

## Quick Start

1. Create local config:

```powershell
Copy-Item .env.example .env
```

2. Edit `.env`:

- optional bootstrap:
  - `AWS_REGION`
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`
  - `AWS_SESSION_TOKEN`
  - `DEFAULT_MODEL_ID`


3. Run:

```powershell
go mod tidy
go run ./cmd/server
```

4. Health check:

```powershell
curl http://127.0.0.1:8080/healthz
```

## Docker Deployment (Linux)

1. Prepare environment file:

```bash
cp .env.example .env
```

2. Edit `.env` .

3. Build image:

```bash
docker build -t aws-cursor-router:latest .
```

4. Run container:

```bash
docker run -d \
  --name aws-cursor-router \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file .env \
  -e DB_PATH=/app/data/router.db \
  -v aws_cursor_router_data:/app/data \
  aws-cursor-router:latest
```

5. Verify service:

```bash
curl http://127.0.0.1:8080/healthz
```

### Docker Compose

Project includes `docker-compose.yml` with persistent SQLite volume.

Start:

```bash
docker compose up -d --build
```

Logs:

```bash
docker compose logs -f
```

Restart:

```bash
docker compose restart
```

Stop:

```bash
docker compose down
```

## Cursor Setup

In Cursor, use OpenAI-compatible custom endpoint:

- Base URL: `http://<server>:8080/v1`
- API Key: one key from your client list
- Model: AWS Bedrock model ID directly (for example `anthropic.claude-3-5-sonnet-20240620-v1:0`)


