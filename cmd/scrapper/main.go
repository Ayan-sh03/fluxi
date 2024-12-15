package main

import (
	"fmt"
	"log"
	"scrapper/internal/api"
	"scrapper/internal/db"
	"scrapper/pkg/logger"

	"github.com/joho/godotenv"
)

// @title           Fluxi Web Scraper API
// @version         1.0
// @description     A powerful and scalable web scraping API service.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@fluxi.com

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8000
// @BasePath  /api/v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

func main() {

	//load .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	logger.Init()
	//initialise DB
	connStr := "postgres://ayan:pgsql123@localhost:5432/postgres?sslmode=disable"
	// connStr := "postgres://ayan:pgsql123@db:5432/postgres?sslmode=disable"
	db, err := db.NewPostgres(connStr)
	if err != nil {
		fmt.Println("Error connecting to DB", err)
		logger.Logger.Fatal(err)
	}
	// err = db.Create() //for sqlite
	// if err != nil {
	// 	logger.Logger.Fatal(err)
	// }

	err = db.Ping()
	if err != nil {
		log.Fatal("Error Pinging to DB ")
		return
	}
	// Start the API server
	api.Run()

}
