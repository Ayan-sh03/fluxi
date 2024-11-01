package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"scrapper/internal/models"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	writeQueue = make(chan writeRequest, 100)
	once       sync.Once
)

func startWriteQueue(db *sql.DB) {
	go func() {
		for req := range writeQueue {
			query := `
                UPDATE scrapped_url
                SET scrapped_urls = ?
                WHERE job_id = ?
            `
			_, err := db.Exec(query, string(req.data), req.jobId)
			req.result <- err
		}
	}()
}

type writeRequest struct {
	jobId  string
	data   []byte
	result chan error
}

type Sqlite struct {
	db *sql.DB
}

func NewSqlite() (*Sqlite, error) {
	db, err := sql.Open("sqlite3", "scrapper.db")
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return &Sqlite{
		db: db,
	}, nil
}

func (db *Sqlite) Create() error {
	query := `
		CREATE TABLE IF NOT EXISTS scrapped_url(
			job_id TEXT PRIMARY KEY,
			scrapped_urls JSON,
      status TEXT CHECK(status IN ('pending', 'completed')) DEFAULT 'pending'
		);
	`
	_, err := db.db.Exec(query)
	return err

}

func (db *Sqlite) UpdateJobStatus(jobId string, status string) error {
	query := `
		UPDATE scrapped_url
		SET status = ?
		WHERE job_id = ?
	`
	_, err := db.db.Exec(query, status, jobId)
	return err
}

func (db *Sqlite) Insert(data models.ScrappedUrl) error {
	// Convert string slice to JSON
	urlsJSON, err := json.Marshal(data.ScrappedUrls)
	if err != nil {
		return err
	}

	query := `
        INSERT INTO scrapped_url (job_id, scrapped_urls)
        VALUES (?, ?)
    `
	_, err = db.db.Exec(query, data.JobId, string(urlsJSON))
	return err
}

func (db *Sqlite) UpdateUrls(jobId string, data []byte) error {
	maxRetries := 5
	backoff := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		tx, err := db.db.Begin()
		if err == nil {
			query := `
                UPDATE scrapped_url
                SET scrapped_urls = ?
                WHERE job_id = ?
            `
			_, err = tx.Exec(query, string(data), jobId)
			if err == nil {
				return tx.Commit()
			}
			tx.Rollback()
		}

		if i < maxRetries-1 {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	return fmt.Errorf("failed to update after %d retries", maxRetries)
}

func (db *Sqlite) Get(jobId string) (models.ScrappedUrl, error) {
	var scrappedUrl models.ScrappedUrl
	var urlsJSON string

	query := `SELECT job_id, scrapped_urls, status  FROM scrapped_url WHERE job_id = ?`
	err := db.db.QueryRow(query, jobId).Scan(&scrappedUrl.JobId, &urlsJSON, &scrappedUrl.Status)
	if err != nil {
		return scrappedUrl, err
	}

	// Parse JSON string back into []string
	err = json.Unmarshal([]byte(urlsJSON), &scrappedUrl.ScrappedUrls)
	if err != nil {
		return scrappedUrl, err
	}

	return scrappedUrl, nil
}

func (db *Sqlite) Delete(jobId string) error {
	query := `
		DELETE FROM scrapped_url
		WHERE job_id = ?
	`
	_, err := db.db.Exec(query, jobId)
	return err
}
func (db *Sqlite) Close() error {
	return db.db.Close()
}
func (db *Sqlite) Ping() error {
	return db.db.Ping()
}

func (db *Sqlite) List(limit, offset int) ([]models.ScrappedUrl, error) {
	query := `
		SELECT job_id, scrapped_urls, status
		FROM scrapped_url
		LIMIT ? OFFSET ?
	`
	rows, err := db.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []models.ScrappedUrl
	for rows.Next() {
		var item models.ScrappedUrl
		err := rows.Scan(&item.JobId, &item.ScrappedUrls, &item.Status)
		if err != nil {
			return nil, err
		}
		data = append(data, item)
	}
	return data, nil
}
