package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/vultr/vcr-credential-provider/pkg/provider"
)

func main() {
	log.SetFlags(0)
	if strings.TrimSpace(os.Getenv("VULTR_API_KEY")) == "" {
		log.Fatal("VULTR_API_KEY must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	driver := provider.NewDriver()
	if err := driver.GetVultrCRCredentialResponse(ctx); err != nil {
		log.Fatal(err)
	}
}
