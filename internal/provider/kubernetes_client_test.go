// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKubeconfigContextAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := `apiVersion: v1
kind: Config
current-context: first
clusters:
- name: first
  cluster:
    server: https://first.example.com
- name: second
  cluster:
    server: https://second.example.com
users:
- name: user
  user:
    token: test-token
contexts:
- name: first
  context:
    cluster: first
    user: user
- name: second
  context:
    cluster: second
    user: user
`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ context, host string }{{"", "https://first.example.com"}, {"second", "https://second.example.com"}} {
		t.Run(test.host, func(t *testing.T) {
			cfg, err := (&kubernetesProviderConfig{ConfigPath: path, ConfigContext: test.context}).restConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Host != test.host || cfg.BearerToken != "test-token" {
				t.Fatal("wrong kubeconfig context or credentials")
			}
		})
	}
	t.Setenv("KUBECONFIG", path)
	cfg, err := (&kubernetesProviderConfig{}).restConfig()
	if err != nil || cfg.Host != "https://first.example.com" {
		t.Fatalf("KUBECONFIG not respected: %v", err)
	}
}

func TestExplicitKubernetesCredentials(t *testing.T) {
	cfg, err := (&kubernetesProviderConfig{Host: "https://cluster.example.com", Token: "token", ClientCertificate: "cert", ClientKey: "key", ClusterCACertificate: "ca"}).restConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://cluster.example.com" || cfg.BearerToken != "token" || string(cfg.CertData) != "cert" || string(cfg.KeyData) != "key" || string(cfg.CAData) != "ca" {
		t.Fatal("explicit credentials not applied")
	}
}
