# Image Credential Provider for VCR

<b>VCR Credential Provider</b> (Provider) for  [Vultr Kubernetes Engine (VKE)](https://www.vultr.com/kubernetes/) is the implementation of [Kubelet CredentialProvider (v1) APIs](https://kubernetes.io/docs/reference/config-api/kubelet-credentialprovider.v1/) for image pulls from the [Vultr Container Registry (VCR)](https://docs.vultr.com/vultr-container-registry). 

This provider will allow Kubelet to obtain access tokens for VCR from the Vultr API which will be provided to the node, so it is able to authenticate without the need of hosted secrets on the cluster. 

## How the Provider Works
The plugin implementation leverages the Kubelet capability introduced in v1.26. Kubelet uses [CredentialProvider](https://kubernetes.io/docs/reference/config-api/kubelet-credentialprovider.v1/) APIs to fetch authentication credentials against Vultr Container Registry and caches it on the worker node level.

The provider is injected into Kubelet via the extra `kubelet-extra-args`:
- `--image-credential-provider-config` sets the path to the Image Credential Provider for VCR config file.
- `--image-credential-provider-bin-dir` sets the path to the directory where the Image Credential Provider for VCR binary is located.

In Managed VKE the flags will be set by the cloud provider(Vultr), however, if you are wanting to use it for self-managed Kubernetes clusters on Vultr you will simply need to add the binary to the worker nodes and specify the path as well as the config file which can be found in the examples directory. 

## Self-managed Kubernetes

Use Kubernetes 1.26 or newer and install the Linux binary on every worker node.
The binary name must match `providers[].name` in [examples/config.yaml](examples/config.yaml).
For example:

```text
--image-credential-provider-config=/etc/kubernetes/credential-provider.yaml
--image-credential-provider-bin-dir=/usr/local/libexec/kubernetes
```

Copy the example configuration to the configured path and replace `ABC123` with
a Vultr API key that can list registries and create Docker credentials. Keep this
file readable only by root (for example, mode `0600`). Restart kubelet after
updating its flags or provider configuration.

Images should use normal container image references, without `https://`:

```text
ewr.vultrcr.com/my-registry/my-image:latest
```

The provider requests read-only credentials with a 12-hour expiry. Kubelet caches
them per image for 11 hours 55 minutes, leaving a margin before expiration.
Credentials are scoped to the VCR registry path, since multiple registries can
share a regional hostname. API work has a 45-second overall deadline and a
30-second HTTP request timeout. Failures go to stderr with a nonzero exit status;
stdout contains only the kubelet JSON response.

## Development

Install Go 1.27.3 or newer, then run:

```sh
make build                 # dist/vcr-credential-provider
make build-linux           # static Linux amd64 binary
make build-linux GOARCH=arm64
make test                  # tests with the race detector
make lint                  # requires golangci-lint v2.15.0
```

Tests use a local mock Vultr API; no API key or cluster is needed. Releases use
GoReleaser v2.19.0 and produce Linux amd64, arm64, and arm binaries. Run
`goreleaser check` to validate the release configuration.

## Contributing

We are open to contributions. If you are wanting to contribute or open a bug please feel free to raise an issue with more information. 

See [GitHub releases](https://github.com/vultr/vcr-credential-provider/releases)
for published versions.
