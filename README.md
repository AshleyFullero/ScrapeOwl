<div align="center">
  <h1>🦉 ScrapeOwl</h1>
  <p><strong>Next-generation web scraping operations platform</strong></p>
  <p>Self-hostable · Browser Automation · AI Extraction · Real-time Dashboard</p>
  
  [![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
  [![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg)](https://golang.org)
  [![Open Source](https://img.shields.io/badge/open--core-OSS-brightgreen.svg)](https://github.com/ashleyfullero/scrapeowl)
</div>

---

ScrapeOwl eliminates the need to manually stitch together browsers, proxies, CAPTCHA solvers, and AI extraction. Define your scraping jobs in simple YAML, and ScrapeOwl handles the rest — with a beautiful real-time dashboard to monitor everything.

## ✨ Features

| Feature | Status |
|---------|--------|
| 🌐 Browser Automation (Click, Type, Scroll, Hover) | ✅ |
| 🎯 CSS & XPath Data Extraction | ✅ |
| 🤖 AI-Powered Extraction (OpenAI / Anthropic) | ✅ |
| 🔄 Proxy Rotation (Static & Rotating Pools) | ✅ |
| 🧩 CAPTCHA Solving (2captcha, AntiCaptcha) | ✅ |
| 📅 Cron Job Scheduling | ✅ |
| 📊 Real-time Dashboard with WebSocket | ✅ |
| 💾 JSONL & CSV Output | ✅ |
| 🔁 Retry with Exponential Backoff | ✅ |
| 🐳 Docker & Docker Compose | ✅ |
| 🔑 Environment Variable Interpolation | ✅ |
| 📋 YAML Job Definitions | ✅ |

## 🚀 Quick Start

### Option 1: Run directly (requires Go 1.22+)

```bash
git clone https://github.com/ashleyfullero/scrapeowl
cd scrapeowl

# Install dependencies and build
go mod download
go build -o ./dist/scrapeowl ./cmd/scrapeowl

# Start the dashboard server
./dist/scrapeowl serve --addr :8080

# Open your browser
open http://localhost:8080
```

### Option 2: Docker

```bash
git clone https://github.com/ashleyfullero/scrapeowl
cd scrapeowl

# Copy and configure environment
cp .env.example .env
# Edit .env with your API keys

# Start with Docker Compose
docker compose up -d

# Open your browser
open http://localhost:8080
```

### Option 3: Single job from CLI

```bash
# Validate a job file
./dist/scrapeowl validate --file ./examples/product-scraper.yaml

# Run a job directly
./dist/scrapeowl run --file ./examples/news-scraper.yaml
```

## 📋 Job Definition

Jobs are defined in YAML files. Here's a complete example:

```yaml
name: "product-scraper"
start_url: "https://example.com/products"

steps:
  - action: click
    selector: "button.load-more"
    wait: 2s
  - action: type
    selector: "input#search"
    text: "laptop"
    wait: 1s

extractors:
  - name: title
    type: css
    selector: "h1.product-title"
    attribute: text
  - name: price
    type: css
    selector: ".price .value"
    attribute: text
  - name: description
    type: ai
    prompt: "Extract the product description. Return JSON with key 'description'."

output:
  format: jsonl          # jsonl or csv
  path: "./output/products.jsonl"

proxy:
  type: static           # static, rotating, or none
  list:
    - "http://user:pass@proxy1:8080"

captcha:
  provider: 2captcha     # 2captcha, anticaptcha, or none
  api_key: "${CAPTCHA_API_KEY}"

ai:
  provider: openai
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o"

retry:
  max_attempts: 3
  backoff: "exponential"

schedule: "0 */6 * * *"  # optional cron expression
```

## 🎯 Supported Actions

| Action | Description | Required Fields |
|--------|-------------|-----------------|
| `click` | Click an element | `selector` |
| `type` | Type text into an input | `selector`, `text` |
| `navigate` | Navigate to a URL | `url` |
| `scroll` | Scroll down or to an element | `selector` (optional) |
| `wait` | Wait for element or time | `selector` or `wait` |
| `hover` | Hover over an element | `selector` |
| `screenshot` | Take a screenshot | `selector` (as path) |
| `select` | Select dropdown option | `selector`, `text` |
| `clear` | Clear an input | `selector` |

## 🔍 Extractor Types

| Type | Description |
|------|-------------|
| `css` | CSS selector extraction |
| `xpath` | XPath expression extraction |
| `ai` | AI-powered extraction with natural language prompt |
| `regex` | Regular expression extraction |

Add `multiple: true` to any extractor to get all matching elements as an array.

## 🔐 Environment Variable Support

Reference environment variables in your YAML with `${VAR_NAME}`:

```yaml
ai:
  api_key: "${OPENAI_API_KEY}"

# With default fallback:
captcha:
  api_key: "${CAPTCHA_API_KEY:-none}"
```

## 🌐 REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/jobs` | List all jobs |
| POST | `/api/jobs` | Create a new job |
| GET | `/api/jobs/{id}` | Get a job |
| PUT | `/api/jobs/{id}` | Update a job |
| DELETE | `/api/jobs/{id}` | Delete a job |
| POST | `/api/jobs/{id}/run` | Start a run |
| GET | `/api/runs` | List runs |
| GET | `/api/runs/{id}` | Get a run |
| POST | `/api/runs/{id}/stop` | Stop a running job |
| GET | `/api/stats` | Platform statistics |
| POST | `/api/validate` | Validate YAML |
| WS | `/ws` | Real-time event stream |

## 🔌 WebSocket Events

Connect to `ws://localhost:8080/ws` for real-time events:

```json
{"type": "log", "level": "info", "message": "Step 1/3: click button.load-more", "job_name": "product-scraper"}
{"type": "step", "data": {"action": "click", "selector": "button.load-more", "success": true}}
{"type": "extract", "data": {"name": "price", "value": "$29.99"}}
{"type": "status", "data": {"status": "success", "progress": 100}}
{"type": "complete", "data": {"records": 1}}
```

Filter by job: `ws://localhost:8080/ws?job=product-scraper`

## 🏗️ Architecture

```
scrapeowl/
├── cmd/scrapeowl/         # CLI entry point (serve, run, validate)
├── internal/
│   ├── config/            # YAML parsing & validation
│   ├── browser/           # Chrome CDP automation (chromedp)
│   ├── proxy/             # Proxy pool management
│   ├── captcha/           # 2captcha & AntiCaptcha
│   ├── extractor/         # CSS/XPath/AI/Regex extraction
│   ├── runner/            # Job orchestration & retry
│   ├── scheduler/         # Cron scheduling
│   ├── output/            # JSONL & CSV writers
│   ├── store/             # SQLite persistence
│   ├── api/               # REST API & WebSocket server
│   └── license/           # Open-core feature flags
└── web/                   # Dashboard (HTML/CSS/JS)
```

## 🔧 Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run with debug logging
make run

# Docker build
make docker
```

## 🛣️ Roadmap

### Open Source (Current)
- ✅ YAML job definitions
- ✅ Browser automation via CDP
- ✅ CSS/XPath/AI/Regex extraction
- ✅ Proxy rotation
- ✅ CAPTCHA solving
- ✅ Cron scheduling
- ✅ Real-time dashboard
- ✅ JSONL/CSV output
- ✅ Docker deployment

### Pro (Coming Soon — Cloud Managed)
- ☁️ Managed cloud execution (pay-per-page)
- 📈 Advanced analytics & reporting
- 🔗 Webhook notifications
- 👥 Team access & collaboration
- 🔑 SSO / Enterprise auth

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

ScrapeOwl is open-source software licensed under the [Apache 2.0 License](LICENSE).

---

<div align="center">
  <p>
    <a href="https://github.com/ashleyfullero/scrapeowl/issues">Report Bug</a> ·
    <a href="https://github.com/ashleyfullero/scrapeowl/issues">Request Feature</a>
  </p>
</div>
