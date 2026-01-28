# nginx-logs-enricher

A lightweight CLI tool to parse Nginx access logs, extract unique client IPs,
and enrich them with geolocation, ASN, and security intelligence using
ipgeolocation.io.

## Features
- Supports IPv4 and IPv6
- Deduplicates IPs to reduce API usage
- Optional security and abuse enrichment
- SQLite-based local caching
- Outputs JSONL for easy analysis
- Docker-first distribution

## Requirements
- Docker (recommended)
- ipgeolocation.io API key

## Quick Start (Docker)

```bash
docker run --rm -it \
  -e IPGEOLOCATION_API_KEY=YOUR_API_KEY \
  -v "$PWD:/data" \
  ipgeolocation/nginx-logs-enricher:latest \
  enrich --input /data/access.log --output /data/enriched.jsonl
