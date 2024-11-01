package db

import (
	"context"
	"scrapper/internal/models"

	"github.com/jackc/pgx/v4/pgxpool"
)

type PostgreSQL struct {
	pool *pgxpool.Pool
}

var Db PostgreSQL

func NewPostgres(connStr string) (*PostgreSQL, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 100 // Configure pool for high concurrency
	config.MinConns = 10  // Keep minimum connections ready

	pool, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	Db = PostgreSQL{pool: pool}
	return &Db, nil
}

func (db *PostgreSQL) Create() error {
	query := `
        CREATE TABLE IF NOT EXISTS scrapped_url(
            job_id TEXT PRIMARY KEY,
            scrapped_urls JSONB,
            status TEXT CHECK(status IN ('pending', 'completed')) DEFAULT 'pending'
        );
    `
	_, err := db.pool.Exec(context.Background(), query)
	return err
}

func (p *PostgreSQL) Insert(data models.ScrappedUrl) error {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(context.Background(),
		"INSERT INTO scrapped_url (job_id, scrapped_urls, status) VALUES ($1, $2, $3)",
		data.JobId, data.ScrappedUrls, data.Status)
	return err
}

func (p *PostgreSQL) Get(id string) (models.ScrappedUrl, error) {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return models.ScrappedUrl{}, err
	}
	defer conn.Release()

	var result models.ScrappedUrl
	err = conn.QueryRow(context.Background(),
		"SELECT job_id, scrapped_urls, status FROM scrapped_url WHERE job_id = $1",
		id).Scan(&result.JobId, &result.ScrappedUrls, &result.Status)

	return result, err
}

func (p *PostgreSQL) List(limit, offset int) ([]models.ScrappedUrl, error) {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(context.Background(),
		"SELECT job_id, scrapped_urls, status FROM scrapped_url LIMIT $1 OFFSET $2",
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.ScrappedUrl
	for rows.Next() {
		var result models.ScrappedUrl
		err := rows.Scan(&result.JobId, &result.ScrappedUrls, &result.Status)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (p *PostgreSQL) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgreSQL) Ping() error {
	return p.pool.Ping(context.Background())
}

func (p *PostgreSQL) UpdateJobStatus(jobId, status string) error {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()
	_, err = conn.Exec(context.Background(),
		"UPDATE scrapped_url SET status = $1 WHERE job_id = $2",
		status, jobId)
	return err
}

func (p *PostgreSQL) UpdateUrls(jobId string, urls []byte) error {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(context.Background(),
		"UPDATE scrapped_url SET scrapped_urls = $1 WHERE job_id = $2",
		string(urls), jobId)
	return err
}
