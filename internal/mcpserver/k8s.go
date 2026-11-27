/*
Copyright 2025 The ClamAV Operator Authors.

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

// Package mcpserver implements a read-only MCP (Model Context Protocol)
// server that exposes the ClamAV Operator custom resources to AI agents.
//
// The server connects to the Kubernetes API with the ambient credentials
// (kubeconfig or in-cluster service account) and speaks the stdio transport,
// so it can be launched directly by MCP clients such as Claude Code or
// Claude Desktop. All logging goes to stderr: stdout is reserved for the
// MCP protocol.
package mcpserver

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/client/config"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
)

// Scheme returns a runtime scheme with the ClamAV operator APIs registered.
// v1beta1 is the storage/hub version; ScanPolicy only exists in v1alpha1.
func Scheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	// v1beta1 first: it is the hub (storage version).
	_ = clamavv1beta1.AddToScheme(scheme)
	_ = clamavv1alpha1.AddToScheme(scheme)
	return scheme
}

// NewClient builds a controller-runtime client using the standard config
// resolution order (handled by controller-runtime):
//
//  1. the --kubeconfig flag (auto-registered by controller-runtime)
//  2. the KUBECONFIG environment variable
//  3. the in-cluster service account config
//  4. $HOME/.kube/config
func NewClient() (client.Client, error) {
	cfg := ctrlcfg.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: Scheme()})
	if err != nil {
		return nil, err
	}
	return c, nil
}
