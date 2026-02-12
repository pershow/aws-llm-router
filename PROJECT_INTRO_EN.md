# AWS Cursor Router - Project Overview

## Project Summary

**AWS Cursor Router** is a high-performance OpenAI-compatible proxy server designed for seamless integration between Cursor IDE and AWS Bedrock. It acts as a middleware proxy, enabling team members to securely access AWS Bedrock's large language model services through unified API keys without directly handling AWS credentials.

## Core Features

### 1. OpenAI-Compatible Interface
- Fully compatible with OpenAI API specifications, supporting `/v1/chat/completions` and `/v1/responses` endpoints
- Seamlessly integrates with Cursor IDE and other OpenAI API-compatible tools
- Supports both streaming and non-streaming responses

### 2. AWS Bedrock Proxy
- Converts OpenAI-formatted requests to AWS Bedrock API calls
- Supports multiple Bedrock models (e.g., Claude 3.5 Sonnet)
- Centralized AWS credential management - team members don't need AWS keys

### 3. Intelligent Tool Calling Support
- Complete support for modern AI coding assistant tool-calling workflows
- Supports `tools`, `tool_choice`, `tool_calls` parameters
- Supports `developer` role (mapped to system prompts)
- Configurable forced tool usage mode (`FORCE_TOOL_USE`)
- Tool argument buffering mechanism to prevent JSON truncation

### 4. Multi-Tenant Management
- API Key-based client authentication
- Per-client configuration:
  - Request rate limiting (RPM)
  - Concurrent request limits
  - Allowed model whitelist
  - Enable/disable status
- SQLite database for persistent configuration and logs

### 5. Request Monitoring & Logging
- Detailed request/response logging
- Complete tool-calling process trace logs
- Configurable debug mode (`DEBUG_REQUESTS`)
- Health check endpoint (`/healthz`)

### 6. Admin Dashboard
- Web-based admin interface (path: `/salessavvy/`)
- Dynamic AWS credential configuration
- Client management (add, edit, delete)
- Model enable/disable controls
- Call log viewer

### 7. Flexible Deployment
- Local development support (`go run`)
- Docker containerization support
- Docker Compose one-command deployment
- Optional TLS reverse proxy feature

## Technical Architecture

### Technology Stack
- **Language**: Go 1.25.7
- **Database**: SQLite (modernc.org/sqlite)
- **AWS SDK**: aws-sdk-go-v2
- **Key Dependencies**:
  - `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` - Bedrock runtime calls
  - `github.com/google/uuid` - UUID generation
  - `golang.org/x/time` - Rate limiting

### Core Modules

```
aws-cursor-router/
鈹溾攢鈹€ cmd/server/          # Main entry point and route definitions
鈹溾攢鈹€ internal/
鈹?  鈹溾攢鈹€ auth/           # API Key authentication and client management
鈹?  鈹溾攢鈹€ bedrockproxy/   # Bedrock API proxy service
鈹?  鈹溾攢鈹€ config/         # Configuration loading and management
鈹?  鈹溾攢鈹€ openai/         # OpenAI protocol data structures
鈹?  鈹斺攢鈹€ store/          # SQLite data storage layer
鈹溾攢鈹€ web/admin/          # Admin dashboard static files (embedded)
鈹斺攢鈹€ data/               # Data directory (SQLite database)
```

## Key Features

### Security
- Server-side centralized AWS credential management
- API Key-based access control
- Client-level permission isolation

### Performance
- High concurrency support (default: 512 concurrent requests)
- Streaming response optimization
- Request rate limiting and concurrency control

### Configurability
- Environment variable configuration support
- Runtime dynamic configuration updates
- Flexible model selection and token limits

### Compatibility
- Full Cursor Agent mode support
- Standard OpenAI SDK compatibility
- CLI/IDE MCP/skills-style tool execution support

## Typical Use Cases

1. **Team AI Coding Collaboration**
   - Unified AWS Bedrock access management
   - Independent API Keys for different team members
   - Usage monitoring and throttling

2. **Cursor IDE Enhancement**
   - Use high-performance AWS Bedrock models instead of OpenAI
   - Full Cursor Agent mode functionality
   - Smooth tool-calling and code editing experience

3. **Cost Control**
   - Centralized AWS resource management
   - Prevent AWS credential leakage
   - Monitor and analyze API call costs

4. **Development & Testing**
   - Quick switching between Bedrock models
   - Debug AI tool-calling workflows
   - Log analysis and troubleshooting

## Configuration Examples

### Cursor IDE Configuration
```
Base URL: http://your-server:8080/v1
API Key: your-client-api-key
Model: anthropic.claude-3-5-sonnet-20240620-v1:0
```

### Environment Variables
```env
LISTEN_ADDR=:8080
AWS_REGION=us-east-1
DEFAULT_MODEL_ID=anthropic.claude-3-5-sonnet-20240620-v1:0
FORCE_TOOL_USE=true
MIN_TOOL_MAX_OUTPUT_TOKENS=8192
DEBUG_REQUESTS=true
```

## Quick Start

### Local Development
```powershell
# 1. Copy configuration file
Copy-Item .env.example .env

# 2. Edit .env with AWS credentials

# 3. Install dependencies and run
go mod tidy
go run ./cmd/server

# 4. Health check
curl http://127.0.0.1:8080/healthz
```

### Docker Deployment
```bash
# Using Docker Compose
docker compose up -d --build

# View logs
docker compose logs -f
```

## Project Advantages

1. **Ready to Use**: Quick deployment with simple configuration
2. **Production Ready**: Comprehensive error handling and logging system
3. **High Performance**: Go implementation with low latency and high throughput
4. **Easy to Maintain**: Clear modular architecture
5. **Feature Complete**: Supports all core features of modern AI assistants
6. **Well Documented**: Detailed troubleshooting guide (TROUBLESHOOTING.md)

## Target Audience

- Development teams using Cursor IDE
- Enterprises requiring unified AWS Bedrock access management
- Developers wanting AWS models with OpenAI-compatible interface
- Project managers needing to monitor and control AI API usage

---

**Project Type**: Enterprise-grade AI Proxy Server  
**Language**: Go  
**License**: (Add based on your license)  
**Status**: Active Development