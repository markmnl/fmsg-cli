// Package config provides application configuration.
package config

import "os"

const (
	DefaultAPIURL = "http://localhost:4930"
	EnvAPIURL     = "FMSG_API_URL"
)

// GetAPIURL returns the API base URL from the environment, or the default.
func GetAPIURL() string {
	if url := os.Getenv(EnvAPIURL); url != "" {
		return url
	}
	return DefaultAPIURL
}
