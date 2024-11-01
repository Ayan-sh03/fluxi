package main

import (
	"log"
	"scrapper/internal/api"
	"scrapper/internal/db"
	"scrapper/pkg/logger"
)

func main() {
	logger.Init()
	//initialise DB
	connStr := "postgres://ayan:pgsql123@localhost:5432/postgres?sslmode=disable"

	db, err := db.NewPostgres(connStr)
	if err != nil {
		logger.Logger.Fatal(err)
	}
	err = db.Create() //for sqlite
	if err != nil {
		logger.Logger.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("Error Pinging to DB ")
		return
	}
	// Start the API server
	api.Run()

}
