// Package config provides application configuration.
package config

import "os"

const (
	DefaultAPIURL = "http://127.0.0.1:8000"
	EnvAPIURL     = "FMSG_API_URL"
	EnvAPIKey     = "FMSG_API_KEY"
)

// GetAPIURL returns the API base URL from the environment, or the default.
func GetAPIURL() string {
	if url := os.Getenv(EnvAPIURL); url != "" {
		return url
	}
	return DefaultAPIURL
}

// GetAPIKey returns the API key from the environment, if set.
func GetAPIKey() string {
	return os.Getenv(EnvAPIKey)
}
