package main

import (
	"context"
	"github.com/vultr/vcr-credential-provider/pkg/provider"
)

func main() {
	driver := provider.NewDriver()

	driver.GetVultrCRCredentialResponse(context.TODO())
}
