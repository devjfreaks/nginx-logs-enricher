package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devjfreaks/nginx-logs-enricher/internal/cache"
	"github.com/devjfreaks/nginx-logs-enricher/internal/dedupe"
	"github.com/devjfreaks/nginx-logs-enricher/internal/enrich"
	"github.com/devjfreaks/nginx-logs-enricher/internal/parser"
	"github.com/spf13/cobra"

	_ "modernc.org/sqlite"
)

var (
	inputPath   string
	outputPath  string
	includeStr  string
	fieldsStr   string
	excludesStr string
	langStr     string
	dbPath      string

	cacheTTLHours int
	cacheMaxMB    int
	dedupeCap     int

	maxEnrich int
	enrichAll bool
	noPrompt  bool

	workers int
	rps     int
)

var enrichCmd = &cobra.Command{
	Use:   "enrich",
	Short: "Stream Nginx logs, enrich IPs with ipgeolocation.io, write JSONL",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := os.Getenv("IPGEOLOCATION_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("IPGEOLOCATION_API_KEY is not set")
		}

		ttlSeconds := int64(cacheTTLHours) * 3600
		maxBytes := int64(cacheMaxMB) * 1024 * 1024

		cacheDB, err := cache.NewSQLiteCache(dbPath)
		if err != nil {
			return err
		}
		defer cacheDB.Close()

		_ = cacheDB.PurgeExpired(ttlSeconds)
		_ = cacheDB.EnforceMaxSize(maxBytes)

		totalUnique, err := countUniqueIPs(inputPath)
		if err != nil {
			return err
		}

		fmt.Printf("Found %d unique IPs in the log.\n", totalUnique)

		limit := totalUnique

		if enrichAll {
			limit = totalUnique
		} else if maxEnrich > 0 {
			if maxEnrich < totalUnique {
				limit = maxEnrich
			} else {
				limit = totalUnique
			}
		} else if !noPrompt && isInteractive() {
			limit, err = askEnrichLimit(totalUnique)
			if err != nil {
				return err
			}
		} else {
			limit = totalUnique
		}

		fmt.Printf("Enriching %d/%d unique IPs...\n", limit, totalUnique)

		client := enrich.NewIPGeolocationClient(apiKey, includeStr, fieldsStr, excludesStr, langStr)

		in, err := os.Open(inputPath)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer out.Close()

		encoder := json.NewEncoder(out)

		seen := dedupe.NewLRUSet(dedupeCap)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 1024*64), 1024*1024)


		type job struct {
			ip string
		}

		type result struct {
			ip        string
			data      any
			cached    bool
			err       error
			apiCalled bool
		}

		jobs := make(chan job, workers*4)
		results := make(chan result, workers*4)

		var (
			linesRead    int64
			uniqueQueued int64
			cacheHits    int64
			apiCalls     int64
		)

		var cacheMu sync.Mutex

		var tokens <-chan time.Time
		if rps > 0 {
			ticker := time.NewTicker(time.Second / time.Duration(rps))
			defer ticker.Stop()
			tokens = ticker.C
		}

		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)

			maintCounter := 0
			for res := range results {
				if res.cached {
					atomic.AddInt64(&cacheHits, 1)
				}
				if res.apiCalled {
					atomic.AddInt64(&apiCalls, 1)
				}

				if res.err != nil {
					_ = encoder.Encode(map[string]any{
						"ip":    res.ip,
						"error": res.err.Error(),
					})
				} else {
					_ = encoder.Encode(map[string]any{
						"ip":     res.ip,
						"data":   res.data,
						"cached": res.cached,
					})
				}

				maintCounter++
				if maintCounter%1000 == 0 {
					cacheMu.Lock()
					_ = cacheDB.PurgeExpired(ttlSeconds)
					_ = cacheDB.EnforceMaxSize(maxBytes)
					cacheMu.Unlock()
				}
			}
		}()

		var wg sync.WaitGroup
		wg.Add(workers)

		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()

				for j := range jobs {
					ip := j.ip

					cacheMu.Lock()
					cached, ok := cacheDB.Get(ip, includeStr, fieldsStr, excludesStr, langStr, ttlSeconds)
					cacheMu.Unlock()

					if ok {
						results <- result{
							ip:     ip,
							data:   cached,
							cached: true,
						}
						continue
					}

					if tokens != nil {
						select {
						case <-ctx.Done():
							results <- result{ip: ip, err: ctx.Err()}
							continue
						case <-tokens:
							// allowed
						}
					} else {
						select {
						case <-ctx.Done():
							results <- result{ip: ip, err: ctx.Err()}
							continue
						default:
						}
					}

					data, err := client.Lookup(ctx, ip)
					if err != nil {
						results <- result{ip: ip, err: err, apiCalled: true}
						continue
					}

					cacheMu.Lock()
					_ = cacheDB.Set(ip, includeStr, fieldsStr, excludesStr, langStr, data)
					cacheMu.Unlock()

					results <- result{
						ip:        ip,
						data:      data,
						cached:    false,
						apiCalled: true,
					}
				}
			}()
		}

		scanErr := make(chan error, 1)
		go func() {
			defer close(jobs)

			for scanner.Scan() {
				atomic.AddInt64(&linesRead, 1)

				ip, ok := parser.ExtractIP(scanner.Text())
				if !ok {
					continue
				}
				if seen.Seen(ip) {
					continue
				}

				cur := atomic.AddInt64(&uniqueQueued, 1)
				if int(cur) > limit {
					break
				}

				select {
				case <-ctx.Done():
					scanErr <- ctx.Err()
					return
				case jobs <- job{ip: ip}:
				}
			}

			if err := scanner.Err(); err != nil {
				scanErr <- err
				return
			}
			scanErr <- nil
		}()

		if err := <-scanErr; err != nil && err != context.DeadlineExceeded {
			wg.Wait()
			close(results)
			<-writerDone
			return err
		}

		wg.Wait()
		close(results)
		<-writerDone

		lines := int(atomic.LoadInt64(&linesRead))
		uniqueOut := int(atomic.LoadInt64(&uniqueQueued))
		cacheHitsVal := int(atomic.LoadInt64(&cacheHits))
		apiCallsVal := int(atomic.LoadInt64(&apiCalls))
		saved := cacheHitsVal

		fmt.Printf(
			"Done.\nLines read: %d\nUnique IPs enriched: %d\nCache hits: %d\nAPI calls: %d\nSaved API calls: %d\nOutput: %s\nCache DB: %s\n",
			lines, uniqueOut, cacheHitsVal, apiCallsVal, saved, outputPath, dbPath,
		)

		return nil
	},
}

func init() {
	enrichCmd.Flags().StringVar(&inputPath, "input", "", "Path to Nginx access log")
	enrichCmd.Flags().StringVar(&outputPath, "output", "enriched.jsonl", "Output JSONL file")
	enrichCmd.Flags().StringVar(&includeStr, "include", "", "Optional include modules, e.g. security,abuse,time_zone")
	enrichCmd.Flags().StringVar(&fieldsStr, "fields", "", "Return only specific fields (comma-separated, supports dot paths)")
	enrichCmd.Flags().StringVar(&excludesStr, "excludes", "", "Exclude fields (comma-separated, supports dot paths)")
	enrichCmd.Flags().StringVar(&langStr, "lang", "", "Language code (paid plans for non-English)")
	enrichCmd.Flags().StringVar(&dbPath, "db", "cache.db", "SQLite cache path")

	enrichCmd.Flags().IntVar(&cacheTTLHours, "cache-ttl-hours", 24, "Cache TTL in hours (0=disable cache, -1=never expire, default 24)")
	enrichCmd.Flags().IntVar(&cacheMaxMB, "cache-max-mb", 200, "Max cache size in MB (default 200)")
	enrichCmd.Flags().IntVar(&dedupeCap, "dedupe-cap", 200000, "In-run dedupe capacity (controls RAM)")

	enrichCmd.Flags().IntVar(&maxEnrich, "max-enrich", 0, "Enrich only N unique IPs (no prompt). Example: --max-enrich 1000")
	enrichCmd.Flags().BoolVar(&enrichAll, "enrich-all", false, "Enrich all unique IPs (no prompt)")
	enrichCmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Never prompt (useful for Docker/CI)")

	enrichCmd.Flags().IntVar(&workers, "workers", 10, "Number of parallel workers (default 10)")
	enrichCmd.Flags().IntVar(&rps, "rps", 5, "Max API requests per second (0 = unlimited). Recommended to avoid rate limits.")

	_ = enrichCmd.MarkFlagRequired("input")
}

func countUniqueIPs(path string) (int, error) {
	tmp, err := os.CreateTemp("", "nginx-unique-*.db")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS seen (ip TEXT PRIMARY KEY);`)
	if err != nil {
		return 0, err
	}

	in, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*64), 1024*1024)

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO seen(ip) VALUES (?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for sc.Scan() {
		ip, ok := parser.ExtractIP(sc.Text())
		if !ok {
			continue
		}
		_, _ = stmt.Exec(ip)
	}
	if err := sc.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM seen`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func askEnrichLimit(total int) (int, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("How many IPs do you want to enrich? Enter a number (e.g. 1000) or 'all': ")
		text, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		text = strings.TrimSpace(strings.ToLower(text))

		if text == "all" || text == "" {
			return total, nil
		}

		n, err := strconv.Atoi(text)
		if err != nil || n <= 0 {
			fmt.Println("Please enter a valid number or 'all'.")
			continue
		}

		if n > total {
			fmt.Printf("You entered %d, but only %d unique IPs exist. Using %d.\n", n, total, total)
			return total, nil
		}

		return n, nil
	}
}
