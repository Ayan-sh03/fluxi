package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"scrapper/pkg/logger"
)

func (w *WriterImpl) Write(url, data string) error {
	//get webhook url from env
	webhookUrl := os.Getenv("WEBHOOK_URL")
	if webhookUrl == "" {
		return fmt.Errorf("WEBHOOK_URL not set")
	}
	//create http client

	client := &http.Client{}
	//create request
	req, err := http.NewRequest("POST", webhookUrl, nil)

	if err != nil {
		return err
	}

	//set headers
	req.Header.Set("Content-Type", "application/json")

	body := struct{ URL, Data string }{url, data}

	//convert data to json
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req.Body = io.NopCloser(bytes.NewBuffer(jsonData))

	//send req
	res, err := client.Do(req)

	logger.Logger.Println("Response Status:", res.Status)

	return err
}
