package main

import (
	"context"
	"flag"
	"log"

	"github.com/Cloady/terraform-provider-cloady/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	debug := flag.Bool("debug", false, "Enable debugger support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/cloady/cloady", Debug: *debug,
	}); err != nil {
		log.Fatal(err)
	}
}
