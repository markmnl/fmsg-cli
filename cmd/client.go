package cmd

import (
	"net/http"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
)

func newAuthenticatedClient() (*api.Client, *auth.Manager) {
	apiURL := config.GetAPIURL()
	manager := auth.NewManager(apiURL)
	client := api.New(apiURL, manager)
	manager.HTTP = &http.Client{
		Transport: client.HTTP.Transport,
		Timeout:   api.DefaultRequestTimeout,
	}
	return client, manager
}
