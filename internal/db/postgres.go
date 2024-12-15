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

func (db *PostgreSQL) CreateUser(user *models.User) error {
	query := `
        INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := db.pool.Exec(context.Background(), query,
		user.ID, user.Name, user.Email, user.PasswordHash,
		user.CreatedAt, user.UpdatedAt)
	return err
}

func (db *PostgreSQL) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	query := `
        SELECT id, name, email, password_hash, created_at, updated_at
        FROM users WHERE email = $1
    `
	err := db.pool.QueryRow(context.Background(), query, email).Scan(
		&user.ID, &user.Name, &user.Email, &user.PasswordHash,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *PostgreSQL) UserExistsByEmail(email string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`
	err := db.pool.QueryRow(context.Background(), query, email).Scan(&exists)
	return exists, err
}

func (db *PostgreSQL) GetAPIKeysByUserID(userId string) ([]models.APIKey, error) {
	var apiKeys []models.APIKey

	query := `
        SELECT id, user_id, api_key, created_at, last_used_at, is_active 
        FROM api_keys 
        WHERE user_id = $1
    `

	rows, err := db.pool.Query(context.Background(), query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var apiKey models.APIKey
		err := rows.Scan(
			&apiKey.ID,
			&apiKey.UserID,
			&apiKey.KeyValue,
			&apiKey.CreatedAt,
			&apiKey.LastUsedAt,
			&apiKey.IsActive,
		)
		if err != nil {
			return nil, err
		}
		apiKeys = append(apiKeys, apiKey)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return apiKeys, nil
}
func (db *PostgreSQL) CreateAPIKey(apiKey *models.APIKey) error {
	query := `INSERT INTO api_keys (id, user_id, key_value, created_at, last_used_at, is_active) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := db.pool.Exec(context.Background(), query,
		apiKey.ID, apiKey.UserID, apiKey.KeyValue, apiKey.CreatedAt, apiKey.LastUsedAt, apiKey.IsActive)
	return err
}

func (db *PostgreSQL) UpdateAPIKeyLastUsedAt(apiKeyID string) error {
	query := `UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`
	_, err := db.pool.Exec(context.Background(), query, apiKeyID)
	return err
}

func (db *PostgreSQL) CheckAPIKeyExists(apiKey string, userId string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM api_keys WHERE key_value = $1 AND is_active = true AND user_id = $2)`
	var exists bool
	err := db.pool.QueryRow(context.Background(), query, apiKey, userId).Scan(&exists)
	return exists, err
}
