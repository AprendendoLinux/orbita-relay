package main

import "orbitarelay/internal/netdet"

func gatewayIPs() []string { return netdet.DockerGateways() }
