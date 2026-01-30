# nginx-logs-enricher

A production-safe CLI tool that reads Nginx access logs, extracts client IP addresses, and enriches them with location, network, and security intelligence using **ipgeolocation.io**.

Built for reliability and efficiency with streaming processing, intelligent caching, bounded memory usage, and **parallel processing** for maximum throughput.

---

## Table of Contents

- [What This Tool Does](#what-this-tool-does)
- [Why Use This Tool](#why-use-this-tool)
- [What Problem Does This Solve](#what-problem-does-this-solve)
- [How It Works](#how-it-works)
- [Output Format](#output-format)
- [Requirements](#requirements)
- [Quick Start (Docker – Recommended)](#quick-start-docker--recommended)
- [Build and Run Locally (Go)](#build-and-run-locally-go)
- [Common Usage Examples](#common-usage-examples)
- [Command Options](#command-options)
- [IPGeolocation.io Plan vs Data Availability](#ipgeolocationio-plan-vs-data-availability)
- [User Control Over API Usage](#user-control-over-api-usage)
- [Non-Interactive Mode (Docker / CI / Automation)](#non-interactive-mode-docker--ci--automation)
- [How Caching Works](#how-caching-works)
- [Memory, Cache, and API Usage (Important to Understand)](#memory-cache-and-api-usage-important-to-understand)
- [Memory Safety](#memory-safety)
- [Parallelism and Performance Tuning](#parallelism-and-performance-tuning)
- [Example Workflows](#example-workflows)
- [Performance Characteristics](#performance-characteristics)
- [Troubleshooting](#troubleshooting)
- [Nginx Log Format Notes](#nginx-log-format-notes)
- [Important Concepts](#important-concepts)
- [License](#license)
- [Support](#support)

---

## What This Tool Does

Given an Nginx access log line like:

```
8.8.8.8 - - [29/Jan/2026:10:00:00 +0000] "GET /login HTTP/1.1" 200
```

This tool will:

1. **Read the log file line by line** (streaming - no memory issues)
2. **Extract each IP address** (IPv4 and IPv6)
3. **Count unique IPs** and show you the total
4. **Ask how many you want to enrich** (gives you control over API usage)
5. **Deduplicate IPs** during the run to avoid redundant API calls
6. **Check the local cache** (`cache.db`)
   - If cached and still valid → reuse cached data
   - If not cached or expired → call ipgeolocation.io API and store the result
7. **Process IPs in parallel** using concurrent workers for maximum throughput
8. **Write structured output** to `enriched.jsonl`

---

## Why Use This Tool

### Production-Safe Design
-  **Streams logs line by line** – works on very large files without loading everything into memory
-  **Bounded memory usage** – won't consume unlimited RAM
-  **24-hour cache by default** – reduces API calls and costs
-  **Capped cache disk usage** – prevents `cache.db` from growing forever
-  **User control over API usage** – you decide how many IPs to enrich
-  **Safe for automation** – non-interactive modes available
-  **Parallel processing** – concurrent workers for faster enrichment
-  **Built-in rate limiting** – prevents API rate limit violations

### IPGeolocation Data
-  Location data (country, city, coordinates)
-  Network information (ASN, ISP, organization)
-  Security signals (VPN, proxy, TOR, bot detection, threat scores)
-  Abuse contact information
-  Timezone and currency data
-  User-agent parsing
-  And more (depending on your API plan)

---

## What Problem Does This Solve

A normal Nginx access log line looks like this:

```
8.8.8.8 - - [29/Jan/2026:10:00:00 +0000] "GET /login HTTP/1.1" 200
```

From this line alone, you cannot easily know:

- ❓ Where the visitor is located
- ❓ Which company or network owns the IP
- ❓ Whether the IP is a proxy, VPN, or bot
- ❓ Whether the IP may be suspicious

**nginx-logs-enricher** solves this by enriching IP addresses using the ipgeolocation.io API and giving you structured output you can analyze.

---

## How It Works

1. Reads your Nginx access log file **line by line**
2. Extracts IPv4 and IPv6 addresses
3. **Counts how many UNIQUE IPs exist** in the log
4. **Shows this number to you**
5. **Asks how many IPs you want to enrich** (or lets you enrich all)
6. Enriches only that many IPs using **parallel workers**
7. Saves results to a local cache to avoid repeated API calls
8. Writes results to an output file in JSON Lines format

---

## Output Format

The tool generates `enriched.jsonl` (JSON Lines format).

Each line represents one processed IP:

```json
{
  "cached": false,
  "data":
  {
    "country_metadata":
    {
      "calling_code": "+1",
      "languages": ["en-US", "es-US", "haw", "fr"],
      "tld": ".us"
    },
    "currency":
    {
      "code": "USD",
      "name": "US Dollar",
      "symbol": "$"
    },
    "ip": "8.8.8.8",
    "location":
    {
      "accuracy_radius": "21.258",
      "city": "Mountain View",
      "confidence": "low",
      "continent_code": "NA",
      "continent_name": "North America",
      "country_capital": "Washington, D.C.",
      "country_code2": "US",
      "country_code3": "USA",
      "country_emoji": "🇺🇸",
      "country_flag": "https://ipgeolocation.io/static/flags/us_64.png",
      "country_name": "United States",
      "country_name_official": "United States of America",
      "district": "Santa Clara",
      "geoname_id": "6301403",
      "is_eu": false,
      "latitude": "37.42240",
      "locality": "Mountain View",
      "longitude": "-122.08421",
      "state_code": "US-CA",
      "state_prov": "California",
      "zipcode": "94043-1351"
    },
    "network":
    {
      "asn":
      {
        "allocation_status": "",
        "as_number": "AS15169",
        "asn_name": "GOOGLE",
        "country": "US",
        "date_allocated": "2012-02-24T00:00",
        "domain": "google.com",
        "num_of_ipv4_routes": "1013",
        "num_of_ipv6_routes": "104",
        "organization": "Google LLC",
        "rir": "ARIN",
        "type": "BUSINESS"
      },
      "company":
      {
        "domain": "google.com",
        "name": "Google LLC",
        "type": "Hosting"
      },
      "connection_type": ""
    }
  },
  "ip": "8.8.8.8"
}
```

**Field meanings:**
- `ip` → The IP address
- `data` → Enrichment data from ipgeolocation.io
- `cached: false` → API was called for this IP
- `cached: true` → Data came from your local cache

---

## Requirements

You need:
- An **ipgeolocation.io API key** ([Get one free](https://ipgeolocation.io))
- Either:
  - **Docker** (recommended, easiest), or
  - **Go 1.20+** (if you want to build locally)

---

## Quick Start (Docker – Recommended)

### Step 1: Prepare your log file

Make sure your Nginx access log exists in the current directory on your host machine:
```bash
ls -lh access.log
```

This directory will be mounted into the container at `/data`.

---

### Step 2: Run the tool
```bash
docker run --rm -it \
  -e IPGEOLOCATION_API_KEY="YOUR_API_KEY" \
  -v "$PWD:/data" \
  devjfreaks/nginx-logs-enricher:latest \
  enrich \
  --input /data/access.log \
  --output /data/enriched.jsonl \
  --db /data/cache.db \
  --workers 10 \
  --rps 5
```

**Explanation:**
* `-v "$PWD:/data"` mounts your current directory into the container at `/data`
* All file paths inside the container must use `/data/...`
* `--workers` controls parallel enrichment (default: 10)
* `--rps` limits API requests per second (recommended to avoid rate limits)

---

### Step 3: Check the output

After the command completes, you will see the following files on your host machine:
```
enriched.jsonl   ← enriched IP data (JSON Lines format)
cache.db         ← local SQLite cache database
```

These files are created in the same directory where you ran the Docker command.

---

### Common Mistake (Avoid This)

❌ **Do not use relative paths inside Docker**, like:
```bash
--input access.log
```

Docker containers cannot see host files unless they are referenced via the mounted path.

✅ **Always use absolute container paths:**
```bash
--input /data/access.log
```

## Build and Run Locally (Go)

### Build the binary

```bash
git clone https://github.com/ipgeolocation/nginx-logs-enricher.git
cd nginx-logs-enricher
go build -o nginx-logs-enricher ./cmd/nginx-logs-enricher
```

### Run it

```bash
export IPGEOLOCATION_API_KEY="YOUR_API_KEY"
./nginx-logs-enricher enrich --input access.log
```

---

## Common Usage Examples

### Basic enrichment (default behavior)

```bash
./nginx-logs-enricher enrich --input access.log
```

### Include security signals

```bash
./nginx-logs-enricher enrich --input access.log --include "security"
```

### Include multiple modules

```bash
./nginx-logs-enricher enrich --input access.log --include "security,abuse,time_zone"
```

### Return only selected fields (smaller output)

```bash
./nginx-logs-enricher enrich \
  --input access.log \
  --fields "location.country_name,location.city,network.asn.organization"
```

### Exclude fields you don't need

```bash
./nginx-logs-enricher enrich \
  --input access.log \
  --excludes "currency,location.country_flag"
```

### High-performance enrichment with parallel workers

```bash
./nginx-logs-enricher enrich \
  --input access.log \
  --workers 20 \
  --rps 10
```

### Advanced cache configuration

```bash
./nginx-logs-enricher enrich \
  --input access.log \
  --cache-ttl-hours 24 \
  --cache-max-mb 100 \
  --dedupe-cap 200000
```

---

## Command Options

### Required

| Flag | Description | Example |
|------|-------------|---------|
| `--input` | Path to your Nginx access log file | `--input /var/log/nginx/access.log` |

### Output

| Flag | Default | Description |
|------|---------|-------------|
| `--output` | `enriched.jsonl` | File where enriched data is written |

### ipgeolocation.io Enrichment

| Flag | Description | Example |
|------|-------------|---------|
| `--include` | Enable extra enrichment modules (comma-separated) | `--include security,abuse` |
| `--fields` | Return only specific fields (supports nested dot paths) | `--fields location.country_name,network.asn` |
| `--excludes` | Remove fields from the response | `--excludes currency,location.country_flag` |
| `--lang` | Response language (non-English requires paid plans) | `--lang fr` |

### Caching and Performance

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | `cache.db` | SQLite cache file path |
| `--cache-ttl-hours` | `24` | How long cached data is valid (0=disable, -1=never expire, >0=hours) |
| `--cache-max-mb` | `200` | Maximum size of cache.db on disk (in MB) |
| `--dedupe-cap` | `200000` | Maximum number of recent IPs remembered per run (controls RAM usage) |

### API Usage Control

| Flag | Description | Example |
|------|-------------|---------|
| `--max-enrich` | Maximum number of IPs to enrich | `--max-enrich 500` |
| `--enrich-all` | Enrich all unique IPs without asking | `--enrich-all` |
| `--no-prompt` | Disable all interactive questions (recommended for Docker, CI, scripts) | `--no-prompt` |

### Parallelism and Rate Limiting

| Flag | Default | Description |
|------|---------|-------------|
| `--workers` | `10` | Number of parallel workers for concurrent API requests |
| `--rps` | `5` | Maximum API requests per second (0 = unlimited, **recommended to avoid rate limits**) |

#### Important Notes:
- When `--no-prompt` is used, the tool will never pause for user input. If neither `--max-enrich` nor `--enrich-all` is provided, the tool will automatically enrich all unique IPs.
- The `--workers` flag controls how many concurrent goroutines process IPs simultaneously, improving throughput for large log files.
- The `--rps` flag prevents hitting API rate limits by controlling the maximum requests per second across all workers. Set to `0` for unlimited (use with caution).
- Increasing `--workers` improves performance but may require a higher `--rps` limit to avoid throttling.

---

## IPGeolocation.io Plan vs Data Availability

This tool uses the **IP Location API** from ipgeolocation.io. The data you receive depends on your subscription plan.

---

### Quick Legend

* ✅ = Available
* ❌ = Not available

---

### High-level Feature Availability

| Feature / Data Type | Free | Standard | Security | Advanced |
|---------------------|------|----------|----------|----------|
| Basic IP Location (country, city, coordinates) | ✅ | ✅ | ✅ | ✅ |
| Location by Domain | ❌ | ✅ | ✅ | ✅ |
| Timezone Information | ✅ | ✅ | ✅ | ✅ |
| Currency Information | ✅ | ✅ | ✅ | ✅ |
| Hostname Information | ❌ | ✅ | ✅ | ✅ |
| Multiple Languages (`--lang`) | ❌ | ✅ | ✅ | ✅ |
| Network / ASN Information | ❌ | ❌ | ❌ | ✅ |
| Organization / ISP | ❌ | ✅ | ✅ | ✅ |
| Security Signals (proxy, tor, bot, spam, attacker) | ❌ | ❌ | ✅ | ✅ |
| Abuse Contact Information | ❌ | ❌ | ❌ | ✅ |
| Company Information | ❌ | ✅ | ✅ | ✅ |
| Priority Support | ❌ | ❌ | ✅ | ✅ |

---

### How This Affects Tool Options

Some command flags only return data if your plan supports it:

* `--include security` → Requires **Security** or **Advanced** plan
* `--include abuse` → Requires **Advanced** plan
* `--include hostname` → Requires **Standard** or higher
* `--include dma` → Requires **Advanced** plan
* `--lang` (non-English responses) → Requires **Standard** or higher

**If a field is not supported by your plan, the API may:**

* Return a warning message, or
* Omit the field entirely

> **This is expected behavior.**

---

### Practical Recommendation

| Plan | Best For |
|------|----------|
| **Free plan** | Basic geolocation (country, city, timezone) |
| **Standard plan** | Analytics, hostname, organization, multi-language data |
| **Security plan** | Proxy, VPN, bot, and threat detection |
| **Advanced plan** | ASN, abuse contacts, and full network intelligence |

---

### Important Note

**This tool does not block or modify any API responses.** It simply forwards what your current ipgeolocation.io plan allows.

If you request fields that your plan does not support, they will not be returned.

---

## User Control Over API Usage

Before enriching, the tool counts how many **UNIQUE IPs** exist and shows you:

```
Found 2400 unique IPs in the log.
```

Then it asks:

```
How many IPs do you want to enrich? (enter a number or 'all')
```

**Examples:**

- Enter `100` → only 100 unique IPs are enriched
- Enter `all` → all IPs are enriched

**This gives you direct control over potential API usage.**

---

## Non-Interactive Mode (Docker / CI / Automation)

If you don't want prompts:

### Enrich only a fixed number:

```bash
./nginx-logs-enricher enrich --input access.log --max-enrich 500 --no-prompt
```

### Enrich everything without asking:

```bash
./nginx-logs-enricher enrich --input access.log --enrich-all --no-prompt
```

### High-speed batch processing:

```bash
./nginx-logs-enricher enrich \
  --input access.log \
  --enrich-all \
  --no-prompt \
  --workers 20 \
  --rps 15
```

---

## How Caching Works

### First run
- Cache is empty
- API calls are made for each unique IP
- `cached` will be `false` in output

### Second run (within 24 hours)
- Cached results are reused
- `cached` will be `true` for most IPs
- API calls are significantly reduced

### Important notes
- If you change `--include`, `--fields`, `--excludes`, or `--lang`, the request is treated as new
- API may be called again even for previously cached IPs
- Cache keys include the IP and enrichment options

---

## Memory, Cache, and API Usage (Important to Understand)

This tool uses two different limits to stay safe and efficient:

* **In-memory dedupe** (`--dedupe-cap`) → controls RAM usage during one run
* **Disk cache** (`--cache-max-mb`) → controls how much data is stored long-term to avoid API calls

They serve different purposes and work together.

---

### `--dedupe-cap` (Memory / RAM)

* Remembers recently processed IPs during the current run only
* Prevents processing the same IP many times in one execution
* Protects RAM from growing endlessly
* Is cleared when the program exits

**If `--dedupe-cap` is set too small for a very large log:**

* Older IPs may be forgotten during the same run
* Those IPs can appear again later in the log
* The tool will process them again

> **Note:** This does not automatically cause API calls, but it increases repeated work.

---

### `--cache-max-mb` (Disk / SQLite Cache)

* Stores enriched IP data across multiple runs
* Is the main protection against repeated API calls
* Has a maximum size on disk
* When the size limit is exceeded, oldest entries are deleted first

#### If you run with:
```bash
--cache-max-mb 100
```

#### And later run with:
```bash
--cache-max-mb 50
```

On the next run:
- The tool detects the cache is larger than 50MB
- **Oldest cached entries are deleted first**
- Cache is reduced safely to stay within the limit
- Nothing breaks – the newest data is kept

This prevents `cache.db` from growing indefinitely.

**If `--cache-max-mb` is set too small:**

* Old IP data may be removed from the cache sooner
* If the same IP appears again later (and is no longer cached):
  * The API will be called again
  * API usage increases

---

### Important Interaction (Why Size Matters)

If you have large log files and set both memory and cache limits too low:

* IPs may be forgotten from memory during the run
* **AND** old IPs may be removed from disk cache
* **Result:** the same IP may trigger a new API call, even if not expired yet.

> **This is expected behavior — not a bug.**

---

### Simple Rule of Thumb

* To reduce API calls → **increase `--cache-max-mb`**
* To reduce repeated processing in one run → **increase `--dedupe-cap`**
* If your data is large and resources allow, use larger values for both

**Defaults are safe for most users, but larger datasets benefit from larger limits.**

---

### One-line Summary

**Memory limits affect how much the tool remembers during one run. Cache limits affect how much the tool remembers across runs and API calls.**

---

## Memory Safety

This tool is designed to handle **very large log files** safely:

-  Never loads the full log file into memory
-  Processes one line at a time (streaming)
-  Uses a bounded in-run dedupe list to prevent RAM growth
-  Configurable dedupe cap (`--dedupe-cap`) controls memory usage
-  Parallel workers share memory efficiently

**Result**: Safe for gigabyte-sized logs on modest hardware.

### How Cache and Memory Work Together

- `--dedupe-cap` controls **MEMORY** during one run
- `--cache-max-mb` controls **DISK** usage across runs
- They do **NOT** conflict with each other
- If limits are exceeded, the tool safely evicts old data
- **Nothing breaks**

---

## Parallelism and Performance Tuning

The tool uses concurrent workers to process multiple IPs simultaneously, dramatically improving throughput.

### How Parallelism Works

1. **Main scanner** reads log file line by line and extracts unique IPs
2. **Job queue** distributes IPs to worker goroutines
3. **Workers** (default: 10) process IPs concurrently:
   - Check cache first
   - Call API if needed (with rate limiting)
   - Store results
4. **Writer** collects results and writes to output file

### Performance Tuning Guidelines

| Scenario | Recommended Settings | Explanation |
|----------|---------------------|-------------|
| **Small logs (<10K IPs)** | `--workers 5 --rps 5` | Default settings work well |
| **Medium logs (10K-100K IPs)** | `--workers 10 --rps 10` | Balanced performance |
| **Large logs (100K-1M IPs)** | `--workers 20 --rps 15` | Higher throughput |
| **Very large logs (>1M IPs)** | `--workers 30 --rps 20` | Maximum parallelism |
| **API rate limit concerns** | `--workers 10 --rps 5` | Conservative approach |
| **Unlimited API plan** | `--workers 50 --rps 0` | Maximum speed (use with caution) |

### Understanding the Flags

**`--workers` (default: 10)**
- Controls how many goroutines process IPs concurrently
- Higher values = faster processing but more memory
- Limited by: RAM, API rate limits, network bandwidth
- Sweet spot: 10-30 for most use cases

**`--rps` (default: 5)**
- Maximum API requests per second across ALL workers
- Prevents hitting ipgeolocation.io rate limits
- Set to `0` for unlimited (dangerous if you have rate limits)
- Check your plan's rate limits before increasing

### Example: High-Performance Configuration

```bash
./nginx-logs-enricher enrich \
  --input large-access.log \
  --workers 25 \
  --rps 20 \
  --cache-max-mb 500 \
  --dedupe-cap 500000 \
  --enrich-all \
  --no-prompt
```

This configuration:
- Uses 25 concurrent workers
- Limits to 20 API requests/second
- Has a large cache (500MB)
- Remembers up to 500K IPs in memory
- Runs without prompts (good for automation)

### Performance Impact

**Without parallelism** (sequential processing):
- ~5 IPs/second
- 1,000 IPs = ~3.3 minutes

**With parallelism** (`--workers 20 --rps 20`):
- ~20 IPs/second (limited by RPS)
- 1,000 IPs = ~50 seconds
- **~4x faster**

**With high cache hit rate** (90%+):
- Only 10% need API calls
- Effective throughput: ~100-200 IPs/second
- 1,000 IPs = ~5-10 seconds
- **~40x faster than first run**

---

## Example Workflows

### Find all VPN/proxy traffic

```bash
# Enrich with security data
./nginx-logs-enricher enrich --input access.log --include security

# Filter for VPN/proxy IPs
cat enriched.jsonl | jq 'select(.data.security.is_proxy == true or .data.security.is_vpn == true)'
```

### Count traffic by country

```bash
cat enriched.jsonl | jq -r '.data.location.country_name' | sort | uniq -c | sort -rn
```

### Find high-threat IPs

```bash
cat enriched.jsonl | jq 'select(.data.security.threat_score > 75)'
```

### Export to CSV for analysis

```bash
cat enriched.jsonl | jq -r '[.ip, .data.location.country_name, .data.network.asn.organization] | @csv' > report.csv
```

### Group by ASN organization

```bash
cat enriched.jsonl | jq -r '.data.network.asn.organization' | sort | uniq -c | sort -rn
```

### Find all bot traffic

```bash
cat enriched.jsonl | jq 'select(.data.security.is_bot == true)'
```

### Fast enrichment of large logs

```bash
# Process 1 million IPs with high performance
./nginx-logs-enricher enrich \
  --input huge-access.log \
  --workers 30 \
  --rps 25 \
  --enrich-all \
  --no-prompt
```

---

## Performance Characteristics

### Processing Speed

**First run (cold cache):**
- Sequential: ~5 IPs/second
- Parallel (10 workers, 5 RPS): ~5 IPs/second (limited by RPS)
- Parallel (20 workers, 20 RPS): ~20 IPs/second

**Subsequent runs (warm cache, 90% hit rate):**
- Sequential: ~50-100 IPs/second
- Parallel (10 workers): ~200-500 IPs/second
- Parallel (30 workers): ~1,000-2,000 IPs/second

### Resource Usage
- **Memory**: Bounded by `--dedupe-cap` (default ~50MB with 200k cap)
- **CPU**: Scales with `--workers` (10-30% per worker)
- **Disk**: Controlled by `--cache-max-mb` (default 200MB)
- **Network**: Limited by `--rps` setting

### Real-World Examples

| Log Size | Unique IPs | Workers | RPS | Cache Hit | Time | Throughput |
|----------|------------|---------|-----|-----------|------|------------|
| 100MB | 5,000 | 10 | 5 | 0% | ~17 min | ~5 IPs/sec |
| 100MB | 5,000 | 10 | 5 | 90% | ~2 min | ~40 IPs/sec |
| 1GB | 50,000 | 20 | 20 | 0% | ~42 min | ~20 IPs/sec |
| 1GB | 50,000 | 20 | 20 | 90% | ~5 min | ~170 IPs/sec |
| 10GB | 500,000 | 30 | 30 | 0% | ~5 hours | ~28 IPs/sec |
| 10GB | 500,000 | 30 | 30 | 90% | ~30 min | ~280 IPs/sec |

---

## Troubleshooting

### `IPGEOLOCATION_API_KEY is not set`

**Solution**: Export your API key before running:

```bash
export IPGEOLOCATION_API_KEY="YOUR_API_KEY"
```

### `Field 'security.is_vpn' is not supported`

**Cause**: Your API plan does not include security features

**Solution**: 
- Upgrade to **Security** or **Advance** plan
- Remove security fields from `--fields` or `--include`

### Fewer IPs in output than log lines

**This is normal**: 
- Logs often contain repeated IPs
- Deduplication and caching are working correctly
- Each unique IP appears once in the output

### Cache growing too large

**Solution**: Adjust cache settings:

```bash
--cache-max-mb 100  # Limit cache to 100MB
--cache-ttl-hours 12  # Expire cache entries after 12 hours
```

Or clear the cache:

```bash
rm cache.db
```

### Why is `cached=false` on first run?

Because the cache is empty and the API must be called.

### What happens if I lower `cache-max-mb` later?

The tool deletes the oldest cached entries until the new limit is respected.

### Slow performance / Low throughput

**Possible causes and solutions:**

1. **Rate limiting** → Increase `--rps` (if your plan allows)
2. **Few workers** → Increase `--workers` (try 20-30)
3. **Network latency** → Check internet connection
4. **API rate limits** → Contact ipgeolocation.io to upgrade plan
5. **Low cache hit rate** → Increase `--cache-ttl-hours` and `--cache-max-mb`

### Getting rate limited by API

**Solutions:**
1. Reduce `--rps` to a lower value
2. Reduce `--workers` to limit concurrency
3. Upgrade your ipgeolocation.io plan
4. Enable and maintain a large cache

```bash
# Conservative settings to avoid rate limits
./nginx-logs-enricher enrich \
  --input access.log \
  --workers 5 \
  --rps 3 \
  --cache-max-mb 500
```

---

## Nginx Log Format Notes

This tool assumes the **client IP is the FIRST field** in each log line (standard Nginx formats).

### Common formats supported

```
# Combined log format
8.8.8.8 - - [29/Jan/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 123 "-" "Mozilla/5.0"

# Common log format
8.8.8.8 - - [29/Jan/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 123
```

### Using X-Forwarded-For or CDN

If your setup uses `X-Forwarded-For` or a CDN, the real client IP may not be the first value.

In that case, you may need to:
- Pre-process your logs to extract the real IP
- Or extend the parser (contributions welcome!)

---

## Important Concepts

### UNIQUE IPs
Logs usually contain many repeated IPs. This tool works with **UNIQUE IPs**, not total log lines.

### STREAMING
The tool does **NOT** load the whole log into memory. It reads one line at a time, so it can handle very large files.

### CACHE
Results are stored locally in a SQLite file called `cache.db`. If an IP was already enriched recently, the API is not called again.

### TTL (TIME TO LIVE)
Cached data is valid for **24 hours** by default. After that, the data is refreshed automatically.

### MEMORY SAFETY
The tool keeps only a limited number of recently seen IPs in memory. This prevents RAM usage from growing endlessly.

### DISK SAFETY
The cache file has a maximum size. If it grows beyond the limit, the oldest entries are deleted.

### PARALLELISM
The tool uses concurrent goroutines (workers) to process multiple IPs simultaneously, dramatically improving performance while respecting rate limits.

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

## Support

- **GitHub Issues**: [https://github.com/ipgeolocation/nginx-logs-enricher/issues](https://github.com/ipgeolocation/nginx-logs-enricher/issues)
- **Email**: support@ipgeolocation.io
- [**IP Location API Documentation**](https://ipgeolocation.io/ip-location-api.html#documentation-overview)
- [**IP Security API Documentation**](https://ipgeolocation.io/ip-security-api.html#documentation-overview)
- [**Pricing**](https://ipgeolocation.io/pricing.html)

---

**Built and maintained by [ipgeolocation.io](https://ipgeolocation.io)**

*Transform raw access logs into actionable intelligence with blazing-fast parallel processing.*