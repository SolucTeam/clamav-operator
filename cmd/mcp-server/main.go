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
// Two transports are supported:
//
//   - stdio (default): a local process launched by the MCP client,
//     authenticated with the ambient kubeconfig or in-cluster credentials.
//
//   - http: a Streamable HTTP endpoint for in-cluster team deployment.
//     Callers authenticate with a Bearer token validated via the
//     Kubernetes TokenReview API; every request then runs with the
//     caller's own RBAC.
//
//     clamav-operator-mcp --transport=http --http-addr=:8080 --auth=tokenreview
//
// On stdio, all logging goes to stderr (stdout carries the MCP protocol).
// On HTTP, logs go to stdout for container log collection.
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
	var transport string
	var httpAddr string
	var authRaw string
	var namespace string

	// NOTE: controller-runtime auto-registers the --kubeconfig flag.
	flag.StringVar(&transport, "transport", "stdio",
		"MCP transport: \"stdio\" (local MCP client) or \"http\" (in-cluster Streamable HTTP)")
	flag.StringVar(&httpAddr, "http-addr", ":8080",
		"listen address in http transport mode")
	flag.StringVar(&authRaw, "auth", "tokenreview",
		"HTTP caller authentication: \"tokenreview\" (Bearer token validated via TokenReview, per-user RBAC) or \"none\" (ambient credentials)")
	flag.StringVar(&namespace, "namespace", "",
		"restrict every tool to this namespace (defaults to all namespaces readable by the caller's RBAC)")
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

	if transport != "stdio" && transport != "http" {
		fmt.Fprintf(os.Stderr, "invalid --transport %q (expected \"stdio\" or \"http\")\n", transport)
		os.Exit(1)
	}
	authMode, err := mcpserver.ParseAuthMode(authRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// stdio: logs on stderr (stdout is the MCP protocol).
	// http: logs on stdout (container log collection).
	logger := zap.New(zap.UseFlagOptions(&opts))
	logf.SetLogger(logger)
	log := logger.WithName("mcp-server")

	mcpserver.SetVersion(mcpVersion)

	k8sClient, cfg, err := mcpserver.NewClientWithConfig()
	if err != nil {
		log.Error(err, "unable to build Kubernetes client")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch transport {
	case "stdio":
		if err := mcpserver.Run(ctx, k8sClient, namespace); err != nil {
			log.Error(err, "MCP server exited with error")
			os.Exit(1)
		}
	case "http":
		if err := mcpserver.RunHTTP(ctx, httpAddr, authMode, k8sClient, cfg, namespace); err != nil {
			log.Error(err, "MCP server exited with error")
			os.Exit(1)
		}
	}
}
