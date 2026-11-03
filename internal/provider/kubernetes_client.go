// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/base64"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type kubernetesProviderConfig struct {
	ConfigPath           string
	KubeconfigBase64     string
	ConfigContext        string
	Host                 string
	Token                string
	ClientCertificate    string
	ClientKey            string
	ClusterCACertificate string
	InClusterConfig      bool
}

func (c *kubernetesProviderConfig) dynamicClient() (dynamic.Interface, error) {
	config, err := c.restConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}

	return client, nil
}

func (c *kubernetesProviderConfig) restConfig() (*rest.Config, error) {
	if c.Host != "" {
		config := &rest.Config{
			Host:        c.Host,
			BearerToken: c.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData:   []byte(c.ClusterCACertificate),
				CertData: []byte(c.ClientCertificate),
				KeyData:  []byte(c.ClientKey),
			},
		}

		return config, nil
	}

	if c.InClusterConfig {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster Kubernetes configuration: %w", err)
		}

		return config, nil
	}

	if c.KubeconfigBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(c.KubeconfigBase64)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig_base64 must contain a valid standard base64-encoded kubeconfig")
		}
		raw, err := clientcmd.Load(decoded)
		if err != nil {
			// Parsing errors may include sensitive kubeconfig contents.
			return nil, fmt.Errorf("unable to parse decoded kubeconfig_base64 as a kubeconfig")
		}
		overrides := &clientcmd.ConfigOverrides{CurrentContext: c.ConfigContext}
		config, err := clientcmd.NewNonInteractiveClientConfig(*raw, c.ConfigContext, overrides, nil).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to load decoded kubeconfig_base64; check its contexts, cluster, and credentials")
		}
		return config, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.ConfigPath != "" {
		loadingRules.ExplicitPath = c.ConfigPath
	}

	overrides := &clientcmd.ConfigOverrides{}
	if c.ConfigContext != "" {
		overrides.CurrentContext = c.ConfigContext
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes configuration: %w", err)
	}

	return config, nil
}
