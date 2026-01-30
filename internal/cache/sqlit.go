package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteCache struct {
	db *sql.DB
}

func NewSQLiteCache(path string) (*SQLiteCache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	c := &SQLiteCache{db: db}

	if err := c.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return c, nil
}

func (c *SQLiteCache) Close() {
	_ = c.db.Close()
}

func (c *SQLiteCache) init() error {

	_, _ = c.db.Exec(`PRAGMA journal_mode=WAL;`)
	_, _ = c.db.Exec(`PRAGMA synchronous=NORMAL;`)
	_, _ = c.db.Exec(`PRAGMA temp_store=MEMORY;`)
	_, _ = c.db.Exec(`PRAGMA foreign_keys=ON;`)


	_, _ = c.db.Exec(`PRAGMA auto_vacuum=INCREMENTAL;`)

	_, err := c.db.Exec(`
	CREATE TABLE IF NOT EXISTS cache (
		ip TEXT,
		include TEXT,
		fields TEXT,
		excludes TEXT,
		lang TEXT,
		data TEXT,
		created INTEGER,
		PRIMARY KEY (ip, include, fields, excludes, lang)
	);`)
	if err != nil {
		return err
	}

	_, _ = c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cache_created ON cache(created);`)
	return nil
}

func (c *SQLiteCache) Get(ip, include, fields, excludes, lang string, ttlSeconds int64) (map[string]any, bool) {
	if ttlSeconds == 0 {
		return nil, false
	}

	var raw string
	var created int64

	err := c.db.QueryRow(
		`SELECT data, created FROM cache
		 WHERE ip=? AND include=? AND fields=? AND excludes=? AND lang=?`,
		ip, include, fields, excludes, lang,
	).Scan(&raw, &created)

	if err != nil {
		return nil, false
	}

	if ttlSeconds > 0 {
		if time.Now().Unix()-created > ttlSeconds {
			return nil, false
		}
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false
	}

	return out, true
}


func (c *SQLiteCache) Set(ip, include, fields, excludes, lang string, data map[string]any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = c.db.Exec(
		`INSERT OR REPLACE INTO cache
		 (ip, include, fields, excludes, lang, data, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ip, include, fields, excludes, lang, string(raw), time.Now().Unix(),
	)
	return err
}

func (c *SQLiteCache) PurgeExpired(ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		return nil
	}
	cutoff := time.Now().Unix() - ttlSeconds
	_, err := c.db.Exec(`DELETE FROM cache WHERE created < ?`, cutoff)
	return err
}

func (c *SQLiteCache) EnforceMaxSize(maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}

	size, err := c.currentSizeBytes()
	if err != nil {
		return err
	}
	if size <= maxBytes {
		return nil
	}

	for tries := 0; tries < 50; tries++ {
		size, _ = c.currentSizeBytes()
		if size <= maxBytes {
			break
		}

		res, err := c.db.Exec(`
			DELETE FROM cache
			WHERE rowid IN (
				SELECT rowid FROM cache
				ORDER BY created ASC
				LIMIT 5000
			)
		`)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			break
		}
	}

	_, _ = c.db.Exec(`PRAGMA incremental_vacuum;`)

	size, _ = c.currentSizeBytes()
	if size > maxBytes {
		return fmt.Errorf("cache still above cap after eviction: %d > %d", size, maxBytes)
	}
	return nil
}

func (c *SQLiteCache) currentSizeBytes() (int64, error) {
	var pageCount int64
	var pageSize int64

	if err := c.db.QueryRow(`PRAGMA page_count;`).Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := c.db.QueryRow(`PRAGMA page_size;`).Scan(&pageSize); err != nil {
		return 0, err
	}
	return pageCount * pageSize, nil
}
