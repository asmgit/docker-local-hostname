//go:build darwin

// Package hostsmanager keeps a managed block in /etc/hosts in sync with running
// docker containers whose hostname ends in the docker-local-hostname domain (env DOCKER_LOCAL_HOSTNAME_DOMAIN,
// default ".ldev"), mapping each hostname to its container IP. It flushes the
// macOS DNS cache when the set changes so updates are visible within ~1s.
//
// This is the host-side name resolution layer; reachability of the container
// IPs is provided by the WireGuard tunnel set up by the main program.
package hostsmanager

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const (
	hostsPath   = "/etc/hosts"
	beginMarker = "# BEGIN DOCKER-LOCAL-HOSTNAME"
	endMarker   = "# END DOCKER-LOCAL-HOSTNAME"
)

func domain() string {
	if d := os.Getenv("DOCKER_LOCAL_HOSTNAME_DOMAIN"); d != "" {
		return d
	}
	return ".ldev"
}

// Run keeps /etc/hosts in sync with *.ldev containers, reconnecting to the
// docker event stream if it drops. It blocks until ctx is cancelled.
func Run(ctx context.Context, cli *client.Client) {
	dom := domain()
	prev := ""

	rebuild := func() {
		entries, ok := collect(ctx, cli, dom)
		if !ok {
			// Docker unreachable: leave /etc/hosts untouched rather than
			// wiping the block during a transient outage.
			return
		}
		if err := writeBlock(entries); err != nil {
			fmt.Fprintf(os.Stderr, "docker-local-hostname: failed to write %s: %v\n", hostsPath, err)
			return
		}
		joined := strings.Join(entries, "\n")
		if joined != prev {
			prev = joined
			flushDNS()
			fmt.Printf("docker-local-hostname: %s set changed (%d entries); flushed DNS\n", dom, len(entries))
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}
		rebuild()

		msgs, errs := cli.Events(ctx, events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("type", "container"),
				filters.Arg("event", "start"),
				filters.Arg("event", "die"),
				filters.Arg("event", "destroy"),
			),
		})

		for loop := true; loop; {
			select {
			case <-ctx.Done():
				return
			case <-errs:
				loop = false
			case <-msgs:
				rebuild()
			}
		}

		time.Sleep(3 * time.Second)
	}
}

// collect returns sorted "IP hostname" lines for running containers whose
// hostname ends in dom. The bool is false when Docker is unreachable, so the
// caller can avoid wiping /etc/hosts during a transient outage.
func collect(ctx context.Context, cli *client.Client, dom string) ([]string, bool) {
	list, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, false
	}

	var out []string
	for _, c := range list {
		info, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil || info.Config == nil {
			continue
		}
		host := info.Config.Hostname
		if !strings.HasSuffix(host, dom) || !validHostname(host) {
			continue
		}
		if ip := lowestIP(info); ip != "" {
			out = append(out, ip+" "+host)
		}
	}
	sort.Strings(out)
	return out, true
}

// validHostname rejects anything that is not a clean DNS label sequence, so a
// container hostname containing whitespace/newlines cannot inject /etc/hosts lines.
func validHostname(h string) bool {
	if h == "" {
		return false
	}
	for _, r := range h {
		switch {
		case r == '-' || r == '.' || r == '_':
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		default:
			return false
		}
	}
	return true
}

// lowestIP returns the numerically lowest IPv4 across the container's networks,
// giving deterministic output across inspect calls (network map order is random).
func lowestIP(info container.InspectResponse) string {
	if info.NetworkSettings == nil {
		return ""
	}
	var ips []net.IP
	for _, n := range info.NetworkSettings.Networks {
		if n == nil || n.IPAddress == "" {
			continue
		}
		if ip := net.ParseIP(n.IPAddress); ip != nil && ip.To4() != nil {
			ips = append(ips, ip.To4())
		}
	}
	if len(ips) == 0 {
		return ""
	}
	sort.Slice(ips, func(i, j int) bool { return bytes.Compare(ips[i], ips[j]) < 0 })
	return ips[0].String()
}

// writeBlock replaces the managed block in /etc/hosts, preserving every other
// line, and publishes it atomically (temp file in the same dir + rename).
func writeBlock(entries []string) error {
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	var kept []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		switch line {
		case beginMarker:
			inBlock = true
		case endMarker:
			inBlock = false
		default:
			if !inBlock {
				kept = append(kept, line)
			}
		}
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}

	var b strings.Builder
	b.WriteString(strings.Join(kept, "\n"))
	b.WriteString("\n")
	b.WriteString(beginMarker + "\n")
	if len(entries) > 0 {
		b.WriteString(strings.Join(entries, "\n") + "\n")
	}
	b.WriteString(endMarker + "\n")

	tmp, err := os.CreateTemp("/etc", "hosts.ldev.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, hostsPath)
}

func flushDNS() {
	_ = exec.Command("dscacheutil", "-flushcache").Run()
	_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
}
