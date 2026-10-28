package main

import (
	"fmt"
	"net/http"
	"time"
)

// checkHealth runs without application initialization so distroless containers
// can probe the API using the application binary itself.
func checkHealth(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d", response.StatusCode)
	}
	return nil
}
