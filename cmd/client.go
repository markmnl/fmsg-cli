package cmd

import (
	"net/http"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
)

func newAuthenticatedClient() (*api.Client, *auth.Manager) {
	apiURL := config.GetAPIURL()
	transport := api.DefaultTransport()

	manager := auth.NewManager(apiURL)
	manager.HTTP = &http.Client{
		Transport: transport,
		Timeout:   api.DefaultRequestTimeout,
	}

	client := api.New(apiURL, manager)
	client.HTTP = &http.Client{
		Transport: transport,
	}
	return client, manager
}
