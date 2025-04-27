# Proxa

An intelligent autoscaler for ASGI/WSGI servers with built-in health monitoring.

## Features

- Automatic worker scaling based on CPU and memory metrics
- Support for multiple ASGI/WSGI servers:
  - Uvicorn
  - Hypercorn
  - Granian
- Built-in health monitoring
- Configurable via CLI or TOML files
- Graceful worker management
- Real-time metrics collection

## Installation

```bash
git clone https://github.com/proxa-server/proxa.git
cd proxa
pip install .
```

## Quick Start
Run your ASGI application with automatic scaling:

With health monitoring enabled:
```
proxa run --server uvicorn --app myapp:app --workers 2
```

## Configuration
### Via TOML File
Create a proxa.toml :

```toml
server = "uvicorn"
app = "myapp:app"
workers = 2
min_workers = 1
max_workers = 5
max_cpu = 80.0
max_mem = 512  # MB
enable_health = true
health_host = "127.0.0.1"
health_port = 9000
 ```

Run with config file:

```bash
proxa run -c proxa.toml
 ```
