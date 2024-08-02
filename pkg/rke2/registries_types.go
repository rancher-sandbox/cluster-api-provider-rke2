/*
Copyright 2023 SUSE.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rke2

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

// Mirror contains the config related to the registry mirror.
type Mirror struct {
	// Endpoints are endpoints for a namespace. CRI plugin will try the endpoints
	// one by one until a working one is found. The endpoint must be a valid url
	// with host specified.
	// The scheme, host and path from the endpoint URL will be used.
	Endpoint []string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`

	// Rewrites are repository rewrite rules for a namespace. When fetching image resources
	// from an endpoint and a key matches the repository via regular expression matching
	// it will be replaced with the corresponding value from the map in the resource request.
	Rewrite map[string]string `json:"rewrite,omitempty" toml:"rewrite" yaml:"rewrite,omitempty"`
}

// AuthConfig contains the config related to authentication to a specific registry.
type AuthConfig struct {
	// Username is the username to login the registry.
	Username string `json:"username,omitempty" toml:"username" yaml:"username,omitempty"`
	// Password is the password to login the registry.
	Password string `json:"password,omitempty" toml:"password" yaml:"password,omitempty"`
	// Auth is a base64 encoded string from the concatenation of the username,
	// a colon, and the password.
	Auth string `json:"auth,omitempty" toml:"auth" yaml:"auth,omitempty"`
	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `json:"identity_token,omitempty" toml:"identitytoken" yaml:"identity_token,omitempty"`
}

// TLSConfig contains the CA/Cert/Key used for a registry.
type TLSConfig struct {
	CAFile             string `json:"ca_file,omitempty"              toml:"ca_file"              yaml:"ca_file,omitempty"`
	CertFile           string `json:"cert_file,omitempty"            toml:"cert_file"            yaml:"cert_file,omitempty"`
	KeyFile            string `json:"key_file,omitempty"             toml:"key_file"             yaml:"key_file,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" toml:"insecure_skip_verify" yaml:"insecure_skip_verify,omitempty"`
}

// Registry is registry settings including mirrors, TLS, and credentials.
type Registry struct {
	// Mirrors are namespace to mirror mapping for all namespaces.
	Mirrors map[string]Mirror `json:"mirrors" toml:"mirrors" yaml:"mirrors"`
	// Configs are configs for each registry.
	// The key is the FDQN or IP of the registry.
	Configs map[string]RegistryConfig `json:"configs" toml:"configs" yaml:"configs"`
}

// RegistryConfig contains configuration used to communicate with the registry.
type RegistryConfig struct {
	// Auth contains information to authenticate to the registry.
	Auth *AuthConfig `json:"auth,omitempty" toml:"auth" yaml:"auth,omitempty"`
	// TLS is a pair of CA/Cert/Key which then are used when creating the transport
	// that communicates with the registry.
	TLS *TLSConfig `json:"tls,omitempty" toml:"tls" yaml:"tls,omitempty"`
}

// RegistryScope is a wrapper around the Registry struct to provide
// the client, context and a logger to the Registry struct.
type RegistryScope struct {
	Registry bootstrapv1.Registry
	Client   client.Client
	Ctx      context.Context
	Logger   logr.Logger
}
