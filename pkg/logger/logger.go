package logger

import (
	"log"
	"os"
)

var Logger *log.Logger

func Init() {
	logFile, err := os.OpenFile("scraperwithCPU.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	Logger = log.New(logFile, "", log.LstdFlags)
	// Also log to stdout
	log.SetOutput(os.Stdout)
}
