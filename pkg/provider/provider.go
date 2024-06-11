package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/vultr/govultr/v3"
	"golang.org/x/oauth2"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
)

type CredentialProviderRequest struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Image      string `json:"image"`
}

type CredentialProviderResponse struct {
	APIVersion    string                `json:"apiVersion"`
	Kind          string                `json:"kind"`
	CacheKeyType  string                `json:"cacheKeyType"`
	CacheDuration string                `json:"cacheDuration"`
	Auth          map[string]AuthConfig `json:"auth"`
}

type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Driver struct {
	Client *govultr.Client
}

type DockerCreds struct {
	Repo map[string]Auth `json:"auths"`
}

type Auth struct {
	Creds string `json:"auth"`
}

func NewDriver() *Driver {
	apiKey := os.Getenv("VULTR_API_KEY")

	config := &oauth2.Config{}
	ctx := context.Background()
	ts := config.TokenSource(ctx, &oauth2.Token{AccessToken: apiKey})
	vultrClient := govultr.NewClient(oauth2.NewClient(ctx, ts))

	_ = vultrClient.SetBaseURL("https://api.vultr.com")
	vultrClient.SetUserAgent("vcr-credential-provider")

	return &Driver{
		Client: vultrClient,
	}
}

func extractRepo(imageName string) string {
	u, err := url.Parse(imageName)
	if err != nil {
		panic(err)
	}

	splitStr := strings.Split(u.Path, "/")

	return splitStr[1]
}

func extractHostname(imageName string) string {
	u, err := url.Parse(imageName)
	if err != nil {
		panic(err)
	}

	return u.Host
}

func readCredentialProviderRequestFromStdin() CredentialProviderRequest {
	var credentialProviderRequest CredentialProviderRequest

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalln(err)
		}
		err = json.Unmarshal(input, &credentialProviderRequest)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		log.Fatalln("provider credential request not supplied to stdin")
	}

	return credentialProviderRequest
}

func (d *Driver) GetVultrCRCredentialResponse(ctx context.Context) {
	expireSecond := 43200
	writeAccess := false
	vcrID := ""

	credentialProviderRequest := readCredentialProviderRequestFromStdin()
	registryName := extractRepo(credentialProviderRequest.Image)
	registryHostname := extractHostname(credentialProviderRequest.Image)

	listOptions := &govultr.ListOptions{PerPage: 300}

	for {
		i, meta, _, err := d.Client.ContainerRegistry.List(ctx, listOptions) //nolint:bodyclose
		if err != nil {
			log.Fatalln(err)
		}

		for _, v := range i { //nolint
			if v.Name == registryName {
				vcrID = v.ID
				break
			}
		}

		if meta.Links.Next == "" {
			break
		}

		listOptions.Cursor = meta.Links.Next
	}

	creds, _, err := d.Client.ContainerRegistry.CreateDockerCredentials(ctx, vcrID, &govultr.DockerCredentialsOpt{
		ExpirySeconds: &expireSecond,
		WriteAccess:   &writeAccess,
	})
	if err != nil {
		panic(err)
	}

	var auths DockerCreds

	err = json.Unmarshal([]byte(creds.String()), &auths)
	if err != nil {
		panic(err)
	}

	authCred := auths.Repo[registryHostname].Creds

	data, err := base64.StdEncoding.DecodeString(authCred)
	if err != nil {
		log.Fatal("error:", err)
	}

	credArr := strings.Split(string(data), ":")

	newResponse := CredentialProviderResponse{
		APIVersion:    "credentialprovider.kubelet.k8s.io/v1",
		Kind:          "CredentialProviderResponse",
		CacheKeyType:  "Registry",
		CacheDuration: "12h",
		Auth: map[string]AuthConfig{
			registryHostname: {
				Username: credArr[0],
				Password: credArr[1],
			},
		},
	}

	result, err := json.Marshal(newResponse)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(result))

}
