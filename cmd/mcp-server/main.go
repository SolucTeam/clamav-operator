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

// Command mcp-server is a read-only MCP (Model Context Protocol) server
// exposing the ClamAV Operator custom resources to AI agents.
//
// It speaks the stdio transport and is designed to be launched directly by
// MCP clients (Claude Code, Claude Desktop, ...):
//
//	clamav-operator-mcp --kubeconfig=/path/to/kubeconfig --namespace=clamav-system
//
// All logging goes to stderr; stdout is reserved for the MCP protocol.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/SolucTeam/clamav-operator/internal/mcpserver"
)

// mcpVersion is overridden at build time via -ldflags.
var mcpVersion = "dev"

func main() {
	var namespace string

	// NOTE: controller-runtime auto-registers the --kubeconfig flag.
	flag.StringVar(&namespace, "namespace", "",
		"restrict every tool to this namespace (defaults to all readable namespaces)")
	showVersion := flag.Bool("version", false, "print version and exit")

	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if *showVersion {
		fmt.Println(mcpVersion)
		return
	}

	// Logs go to stderr; stdout carries the MCP stdio protocol.
	logger := zap.New(zap.UseFlagOptions(&opts))
	logf.SetLogger(logger)
	log := logger.WithName("mcp-server")

	mcpserver.SetVersion(mcpVersion)

	k8sClient, err := mcpserver.NewClient()
	if err != nil {
		log.Error(err, "unable to build Kubernetes client")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mcpserver.Run(ctx, k8sClient, namespace); err != nil {
		log.Error(err, "MCP server exited with error")
		os.Exit(1)
	}
}
