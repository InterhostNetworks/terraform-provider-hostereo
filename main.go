package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/InterhostNetworks/terraform-provider-hostereo/internal/provider"
)

// version is set by goreleaser at build time.
var version = "dev"

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name hostereo

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/interhostnetworks/hostereo",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
