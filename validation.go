package goichi

import (
	"errors"
	"fmt"
)

// ValidateConfig checks the application configuration for required fields
// and potential misconfigurations (fail-fast).
func (a *App) ValidateConfig() error {
	var errs []error

	if a.Config.Server.AppName == "" {
		errs = append(errs, errors.New("appName is required"))
	}

	if a.Config.JWT.Secret == "" && a.Config.JWT.TokenLookup != "" {
		errs = append(errs, errors.New("jwt secret is required when jwt auth is configured"))
	}

	if a.Config.Server.Multiprocess {
		if a.Config.Server.NumChildren < 0 {
			errs = append(errs, errors.New("numChildren cannot be negative"))
		}
	}

	if a.Config.Docs.Enable && a.Config.Docs.Path != "" && a.Config.Docs.Path[0] != '/' {
		errs = append(errs, errors.New("docs path must start with '/'"))
	}

	if a.Config.Server.EnablePortMultiplexing {
		if len(a.GetProtocols().GetAll()) == 0 {
			errs = append(errs, errors.New("at least one protocol must be registered when port multiplexing is enabled"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed: %v", errs)
	}
	return nil
}
