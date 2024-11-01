package db

type DB interface {
	Insert(data interface{}) error
	// Update(id string, data interface{}) error
	Delete(id string) error
	Get(id string) (interface{}, error)
	List(limit, offset int) ([]interface{}, error)
	Close() error
	Ping() error
	UpdateUrls()
}
