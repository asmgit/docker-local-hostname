package networkmanager

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/network"
)

type NetworkManager struct {
	mu             sync.Mutex
	DockerNetworks map[string]network.Inspect
}

func New() NetworkManager {
	return NetworkManager{
		DockerNetworks: map[string]network.Inspect{},
	}
}

// SetInterfaceAddress Set the point-to-point IP address configuration on a network interface.
func (manager *NetworkManager) SetInterfaceAddress(ip string, peerIp string, iface string) (string, string, error) {

	cmd := exec.Command("ifconfig", iface, "inet", ip+"/32", peerIp)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

// AddRoute Add a route to the macOS routing table.
func (manager *NetworkManager) AddRoute(net string, iface string) (string, string, error) {

	cmd := exec.Command("route", "-q", "-n", "add", "-inet", net, "-interface", iface)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

// DeleteRoute Delete a route from the macOS routing table.
func (manager *NetworkManager) DeleteRoute(net string) (string, string, error) {

	cmd := exec.Command("route", "-q", "-n", "delete", "-inet", net)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func (manager *NetworkManager) ProcessDockerNetworkCreate(network network.Inspect, iface string) {
	manager.mu.Lock()
	manager.DockerNetworks[network.ID] = network
	manager.mu.Unlock()

	for _, config := range network.IPAM.Config {
		if network.Scope == "local" {
			fmt.Printf("Adding route for %s -> %s (%s)\n", config.Subnet, iface, network.Name)

			_, stderr, err := manager.AddRoute(config.Subnet, iface)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to add route: %v. %v\n", err, stderr)
			}
		}
	}
}

func (manager *NetworkManager) ProcessDockerNetworkDestroy(network network.Inspect) {
	for _, config := range network.IPAM.Config {
		if network.Scope == "local" {
			fmt.Printf("Deleting route for %s (%s)\n", config.Subnet, network.Name)

			_, stderr, err := manager.DeleteRoute(config.Subnet)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete route: %v. %v\n", err, stderr)
			}
		}
	}
	manager.mu.Lock()
	delete(manager.DockerNetworks, network.ID)
	manager.mu.Unlock()
}

func (manager *NetworkManager) DesiredSubnets() []string {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	var subnets []string
	for _, net := range manager.DockerNetworks {
		if net.Scope != "local" {
			continue
		}
		for _, config := range net.IPAM.Config {
			if config.Subnet != "" {
				subnets = append(subnets, config.Subnet)
			}
		}
	}
	return subnets
}

func (manager *NetworkManager) RouteExists(subnet string, iface string) bool {
	host := subnet
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}

	cmd := exec.Command("route", "-n", "get", "-inet", host)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "interface:")) == iface
		}
	}
	return false
}

func (manager *NetworkManager) EnsureRoutes(iface string) int {
	added := 0
	for _, subnet := range manager.DesiredSubnets() {
		if manager.RouteExists(subnet, iface) {
			continue
		}
		if _, stderr, err := manager.AddRoute(subnet, iface); err != nil {
			fmt.Fprintf(os.Stderr, "reconcile: failed to add route %s -> %s: %v. %v\n", subnet, iface, err, stderr)
			continue
		}
		added++
	}
	return added
}

func TunnelStale(deviceErr bool, lastHandshake time.Time, now time.Time, threshold time.Duration) bool {
	if deviceErr {
		return true
	}
	if lastHandshake.IsZero() {
		return true
	}
	return now.Sub(lastHandshake) > threshold
}
