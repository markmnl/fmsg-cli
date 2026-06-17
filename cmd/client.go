package cmd

import (
	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
)

func newAuthenticatedClient() (*api.Client, *auth.Manager) {
	apiURL := config.GetAPIURL()
	manager := auth.NewManager(apiURL)
	return api.New(apiURL, manager), manager
}
