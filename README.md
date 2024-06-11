# Image Credential Provider for VCR

<b>VCR Credential Provider</b> (Provider) for  [Vultr Kubernetes Engine (VKE)](https://www.vultr.com/kubernetes/) is the implementation of [Kubelet CredentialProvider (v1) APIs](https://kubernetes.io/docs/reference/config-api/kubelet-credentialprovider.v1/) for image pulls from the [Vultr Container Registry (VCR)](https://docs.vultr.com/vultr-container-registry). 

This provider will allow Kubelet to obtain access tokens for VCR from the Vultr API which will be provided to the node, so it is able to authenticate without the need of hosted secrets on the cluster. 

## How the Provider Works
The plugin implementation leverages the Kubelet capability introduced in v1.26. Kubelet uses [CredentialProvider](https://kubernetes.io/docs/reference/config-api/kubelet-credentialprovider.v1/) APIs to fetch authentication credentials against Vultr Container Registry and caches it on the worker node level.

The provider is injected into Kubelet via the extra `kubelet-extra-args`:
- `--image-credential-provider-config` sets the path to the Image Credential Provider for VCP config file.
- `--image-credential-provider-bin-dir` sets the path to the directory where the Image Credential Provider for VCP binary is located.

In Managed VKE the flags will be set by the cloud provider(Vultr), however, if you are wanting to use it for self-managed Kubernetes clusters on Vultr you will simply need to add the binary to the worker nodes and specify the path as well as the config file which can be found in the examples directory. 

## Contributing

We are open to contributions. If you are wanting to contribute or open a bug please feel free to raise an issue with more information. 

Current Version: v0.0.1
