package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vultr/govultr/v3"
	"golang.org/x/oauth2"
)

const apiVersion = "credentialprovider.kubelet.k8s.io/v1"

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
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("VULTR_API_KEY")})
	client := oauth2.NewClient(context.Background(), ts)
	client.Timeout = 30 * time.Second
	vultrClient := govultr.NewClient(client)
	vultrClient.SetUserAgent("vcr-credential-provider")
	return &Driver{Client: vultrClient}
}

// parseImage handles kubelet image references (host/registry/repository:tag),
// which are not URLs. Only VCR hosts may receive the returned credentials.
func parseImage(image string) (host, registry string, err error) {
	parts := strings.SplitN(image, "/", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" ||
		strings.ContainsAny(image, " \t\r\n?#\\") || strings.Contains(image, "://") {
		return "", "", fmt.Errorf("expected a VCR image in host/registry/repository format")
	}
	u, err := url.Parse("https://" + parts[0])
	if err != nil || u.User != nil || u.Host != parts[0] || u.Port() != "" ||
		!strings.HasSuffix(parts[0], ".vultrcr.com") || strings.TrimSuffix(parts[0], ".vultrcr.com") == "" ||
		strings.ContainsAny(parts[1], ":@*") || parts[1] == "." || parts[1] == ".." {
		return "", "", fmt.Errorf("invalid VCR host or registry")
	}
	return parts[0], parts[1], nil
}

// Run reads one kubelet request and writes only the JSON response to output.
func (d *Driver) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	var request CredentialProviderRequest
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("decode credential provider request: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one credential provider request")
	}
	response, err := d.credentials(ctx, request)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(response); err != nil {
		return fmt.Errorf("encode credential provider response: %w", err)
	}
	return nil
}

func (d *Driver) GetVultrCRCredentialResponse(ctx context.Context) error {
	return d.Run(ctx, os.Stdin, os.Stdout)
}

func (d *Driver) credentials(ctx context.Context, request CredentialProviderRequest) (*CredentialProviderResponse, error) {
	if request.APIVersion != apiVersion || request.Kind != "CredentialProviderRequest" {
		return nil, fmt.Errorf("expected %s CredentialProviderRequest", apiVersion)
	}
	host, registry, err := parseImage(request.Image)
	if err != nil {
		return nil, err
	}
	id, err := d.registryID(ctx, registry)
	if err != nil {
		return nil, err
	}

	// TODO: govultr v3.33.2 serializes the ExpirySeconds pointer rather than
	// its value. Keep using the SDK pending an upstream fix.
	expirySeconds := 43200
	writeAccess := false
	creds, _, err := d.Client.ContainerRegistry.CreateDockerCredentials(ctx, id, &govultr.DockerCredentialsOpt{
		ExpirySeconds: &expirySeconds,
		WriteAccess:   &writeAccess,
	})
	if err != nil {
		return nil, fmt.Errorf("create registry credentials: %w", err)
	}
	var auths DockerCreds
	if creds == nil {
		return nil, fmt.Errorf("empty registry credentials response")
	}
	if err := json.Unmarshal([]byte(*creds), &auths); err != nil {
		return nil, fmt.Errorf("invalid registry credentials JSON")
	}
	auth, ok := auths.Repo[host]
	if !ok || auth.Creds == "" {
		return nil, fmt.Errorf("credentials response has no auth for %s", host)
	}
	data, err := base64.StdEncoding.DecodeString(auth.Creds)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 registry credentials")
	}
	username, password, ok := strings.Cut(string(data), ":")
	if !ok || username == "" || password == "" {
		return nil, fmt.Errorf("registry credentials must contain a non-empty username and password")
	}
	return &CredentialProviderResponse{
		APIVersion: apiVersion,
		Kind:       "CredentialProviderResponse",
		// VCR namespaces share a hostname but require different credentials.
		CacheKeyType: "Image",
		// Leave a margin before the API's 12-hour credential expiry.
		CacheDuration: "11h55m",
		Auth: map[string]AuthConfig{
			host + "/" + registry: {Username: username, Password: password},
		},
	}, nil
}

func (d *Driver) registryID(ctx context.Context, name string) (string, error) {
	options := &govultr.ListOptions{PerPage: 300}
	seen := make(map[string]bool)
	for {
		registries, meta, _, err := d.Client.ContainerRegistry.List(ctx, options)
		if err != nil {
			return "", fmt.Errorf("list registries: %w", err)
		}
		for _, registry := range registries {
			if registry.Name == name && registry.ID != "" {
				return registry.ID, nil
			}
		}
		if meta == nil || meta.Links == nil || meta.Links.Next == "" {
			return "", fmt.Errorf("VCR registry %q not found", name)
		}
		if seen[meta.Links.Next] {
			return "", fmt.Errorf("registry API returned a repeated pagination cursor")
		}
		options.Cursor = meta.Links.Next
		seen[options.Cursor] = true
	}
}
