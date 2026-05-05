//go:build wireinject
// +build wireinject

// Wire provider setup. `make wire` regenerates wire_gen.go from this file.
package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	dataProviders "github.com/menta2k/iris/backend/app/admin/service/internal/data/providers"
	serverProviders "github.com/menta2k/iris/backend/app/admin/service/internal/server/providers"
	serviceProviders "github.com/menta2k/iris/backend/app/admin/service/internal/service/providers"
)

func initApp(*bootstrap.Context) (*kratos.App, func(), error) {
	panic(wire.Build(
		dataProviders.ProviderSet,
		serviceProviders.ProviderSet,
		serverProviders.ProviderSet,
		newApp,
	))
}
