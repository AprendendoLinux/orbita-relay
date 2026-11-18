package main

import "orbitarelay/internal/netdet"

func ifaceNets() []string  { return netdet.InterfaceNets() }
func gatewayIPs() []string { return netdet.DockerGateways() }
