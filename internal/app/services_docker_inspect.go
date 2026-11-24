package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type dockerInspectHealth struct {
	Status string `json:"Status"`
}

type dockerContainerInspectState struct {
	Status     string               `json:"Status"`
	Running    bool                 `json:"Running"`
	Paused     bool                 `json:"Paused"`
	Restarting bool                 `json:"Restarting"`
	OOMKilled  bool                 `json:"OOMKilled"`
	Dead       bool                 `json:"Dead"`
	Pid        int                  `json:"Pid"`
	ExitCode   int                  `json:"ExitCode"`
	Error      string               `json:"Error"`
	StartedAt  string               `json:"StartedAt"`
	FinishedAt string               `json:"FinishedAt"`
	Health     *dockerInspectHealth `json:"Health"`
}

type dockerInspectConfig struct {
	Hostname     string                     `json:"Hostname"`
	User         string                     `json:"User"`
	Env          []string                   `json:"Env"`
	Cmd          []string                   `json:"Cmd"`
	Image        string                     `json:"Image"`
	WorkingDir   string                     `json:"WorkingDir"`
	Entrypoint   []string                   `json:"Entrypoint"`
	Labels       map[string]string          `json:"Labels"`
	ExposedPorts map[string]json.RawMessage `json:"ExposedPorts"`
}

type dockerInspectPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type dockerInspectNetworkAttachment struct {
	IPAddress         string `json:"IPAddress"`
	GlobalIPv6Address string `json:"GlobalIPv6Address"`
	Gateway           string `json:"Gateway"`
	MacAddress        string `json:"MacAddress"`
}

type dockerInspectNetworkSettings struct {
	Ports    map[string][]dockerInspectPortBinding     `json:"Ports"`
	Networks map[string]dockerInspectNetworkAttachment `json:"Networks"`
}

type dockerInspectMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

type dockerContainerInspect struct {
	ID         string                      `json:"Id"`
	Name       string                      `json:"Name"`
	Created    string                      `json:"Created"`
	Path       string                      `json:"Path"`
	Args       []string                    `json:"Args"`
	Image      string                      `json:"Image"`
	State      dockerContainerInspectState `json:"State"`
	Config     dockerInspectConfig         `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	NetworkSettings dockerInspectNetworkSettings `json:"NetworkSettings"`
	Mounts          []dockerInspectMount         `json:"Mounts"`
}

type dockerImageInspect struct {
	ID           string              `json:"Id"`
	RepoTags     []string            `json:"RepoTags"`
	RepoDigests  []string            `json:"RepoDigests"`
	Created      string              `json:"Created"`
	Architecture string              `json:"Architecture"`
	OS           string              `json:"Os"`
	Size         int64               `json:"Size"`
	Config       dockerInspectConfig `json:"Config"`
}

type dockerNetworkIPAMConfig struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

type dockerNetworkContainer struct {
	Name        string `json:"Name"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
	MacAddress  string `json:"MacAddress"`
}

type dockerNetworkInspect struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Created    string `json:"Created"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	EnableIPv6 bool   `json:"EnableIPv6"`
	Internal   bool   `json:"Internal"`
	IPAM       struct {
		Config []dockerNetworkIPAMConfig `json:"Config"`
	} `json:"IPAM"`
	Containers map[string]dockerNetworkContainer `json:"Containers"`
	Labels     map[string]string                 `json:"Labels"`
}

func dockerContainerInspectFromText(text string) *dockerContainerInspect {
	var value dockerContainerInspect
	if json.Unmarshal([]byte(text), &value) != nil || value.ID == "" {
		return nil
	}
	return &value
}

func dockerImageInspectFromText(text string) *dockerImageInspect {
	var value dockerImageInspect
	if json.Unmarshal([]byte(text), &value) != nil || value.ID == "" {
		return nil
	}
	return &value
}

func dockerNetworkInspectFromText(text string) *dockerNetworkInspect {
	var value dockerNetworkInspect
	if json.Unmarshal([]byte(text), &value) != nil || value.ID == "" {
		return nil
	}
	return &value
}

func dockerInspectCommand(entrypoint, cmd []string, fallback string) string {
	parts := append([]string(nil), entrypoint...)
	parts = append(parts, cmd...)
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, " ")
}

func dockerEnvRows(env []string) [][2]string {
	rows := make([][2]string, 0, len(env))
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			value = ""
		}
		rows = append(rows, [2]string{key, value})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	return rows
}

func dockerExposedPorts(ports map[string]json.RawMessage) []string {
	out := make([]string, 0, len(ports))
	for port := range ports {
		out = append(out, port)
	}
	sort.Strings(out)
	return out
}

func dockerInspectPortMappings(
	ports map[string][]dockerInspectPortBinding,
	fallback string,
) []dockerPortMapping {
	seen := map[string]bool{}
	out := make([]dockerPortMapping, 0)
	add := func(hostIP, hostPort, containerPort, protocol string) {
		key := hostIP + ":" + hostPort + "->" + containerPort + "/" + protocol
		if seen[key] {
			return
		}
		seen[key] = true
		browseHost := hostIP
		if browseHost == "" || browseHost == "0.0.0.0" || browseHost == "::" || browseHost == "[::]" {
			browseHost = "localhost"
		}
		url := ""
		if hostPort != "" && protocol == "tcp" {
			url = "http://" + browseHost + ":" + hostPort
		}
		label := containerPort + "/" + protocol
		if hostPort != "" {
			shownIP := hostIP
			if shownIP == "" {
				shownIP = "0.0.0.0"
			}
			label = shownIP + ":" + hostPort + " -> " + containerPort + "/" + protocol
		}
		out = append(out, dockerPortMapping{
			hostIP: hostIP, hostPort: hostPort, containerPort: containerPort,
			protocol: protocol, url: url, label: label,
		})
	}
	for spec, bindings := range ports {
		containerPort, protocol, _ := strings.Cut(spec, "/")
		if protocol == "" {
			protocol = "tcp"
		}
		if len(bindings) == 0 {
			add("", "", containerPort, protocol)
			continue
		}
		for _, binding := range bindings {
			add(binding.HostIP, binding.HostPort, containerPort, protocol)
		}
	}
	if len(out) == 0 {
		return dockerPortMappings(fallback)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].label < out[j].label })
	return out
}

func dockerBytes(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func dockerNetworkIPAMText(network *dockerNetworkInspect) string {
	if network == nil {
		return ""
	}
	parts := make([]string, 0, len(network.IPAM.Config))
	for _, config := range network.IPAM.Config {
		value := config.Subnet
		if config.Gateway != "" {
			if value != "" {
				value += " · gw "
			}
			value += config.Gateway
		}
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ", ")
}

func dockerNetworkContainerAddress(network *dockerNetworkInspect, containerID string) string {
	if network == nil {
		return ""
	}
	entry, ok := network.Containers[containerID]
	if !ok {
		return ""
	}
	switch {
	case entry.IPv4Address != "":
		return entry.IPv4Address
	case entry.IPv6Address != "":
		return entry.IPv6Address
	default:
		return entry.MacAddress
	}
}

func (m *Model) dockerContainerInspect(
	connectionID, id string,
) *dockerContainerInspect {
	state := m.services.docker.inspect[dockerDetailKey(connectionID, "container", id)]
	if state == nil || state.loading || state.err != "" || state.text == "" {
		return nil
	}
	return dockerContainerInspectFromText(state.text)
}

func (m *Model) dockerImageInspect(
	connectionID, id string,
) *dockerImageInspect {
	state := m.services.docker.inspect[dockerDetailKey(connectionID, "image", id)]
	if state == nil || state.loading || state.err != "" || state.text == "" {
		return nil
	}
	return dockerImageInspectFromText(state.text)
}

func (m *Model) dockerNetworkInspect(
	node serviceTreeNode, id string,
) *dockerNetworkInspect {
	state := m.services.docker.inspect[dockerDetailKey(node.dockerConn, "network", id)]
	if state == nil || state.loading || state.err != "" || state.text == "" {
		return nil
	}
	return dockerNetworkInspectFromText(state.text)
}

func (m *Model) dockerContainerPortBindings(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer,
) {
	var ports []dockerPortMapping
	if inspect := m.dockerContainerInspect(node.dockerConn, container.ID); inspect != nil {
		ports = dockerInspectPortMappings(inspect.NetworkSettings.Ports, container.Ports)
	}
	if len(ports) == 0 && container.Ports != "" {
		ports = dockerPortMappings(container.Ports)
	}
	ui.Text(c, "Port Bindings").FontSize(12).Bold().TextColor(p.Text)
	if len(ports) == 0 {
		ui.Text(c, "No port bindings").FontSize(11).TextColor(p.TextMuted)
		return
	}
	m.dockerPortMappingRows(c, p, ports)
}

func (m *Model) dockerContainerInspectDetails(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer,
) {
	inspect := m.dockerContainerInspect(node.dockerConn, container.ID)
	if inspect == nil {
		return
	}
	if inspect.State.Status != "" {
		serviceFact(c, p, "Runtime state", inspect.State.Status, false)
	}
	if inspect.State.Health != nil && inspect.State.Health.Status != "" {
		serviceFact(c, p, "Health", inspect.State.Health.Status, false)
	}
	if inspect.State.Pid > 0 {
		serviceFact(c, p, "PID", strconv.Itoa(inspect.State.Pid), true)
	}
	if !inspect.State.Running {
		serviceFact(c, p, "Exit code", strconv.Itoa(inspect.State.ExitCode), true)
	}
	if inspect.State.Error != "" {
		serviceFact(c, p, "State error", inspect.State.Error, true)
	}
	if inspect.State.StartedAt != "" && !strings.HasPrefix(inspect.State.StartedAt, "0001-") {
		serviceFact(c, p, "Started", inspect.State.StartedAt, true)
	}
	if inspect.State.FinishedAt != "" && !strings.HasPrefix(inspect.State.FinishedAt, "0001-") {
		serviceFact(c, p, "Finished", inspect.State.FinishedAt, true)
	}
	if inspect.HostConfig.RestartPolicy.Name != "" {
		serviceFact(c, p, "Restart policy", inspect.HostConfig.RestartPolicy.Name, false)
	}
	if inspect.Config.User != "" {
		serviceFact(c, p, "User", inspect.Config.User, true)
	}
	if inspect.Config.WorkingDir != "" {
		serviceFact(c, p, "Working directory", inspect.Config.WorkingDir, true)
	}
	if command := dockerInspectCommand(inspect.Config.Entrypoint, inspect.Config.Cmd, ""); command != "" {
		serviceFact(c, p, "Resolved command", command, true)
	}
}

func (m *Model) dockerContainerMountsView(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer,
) {
	inspect := m.dockerContainerInspect(node.dockerConn, container.ID)
	ui.Text(c, "Volume & Bind Mounts").FontSize(12).Bold().TextColor(p.Text)
	if inspect != nil && len(inspect.Mounts) > 0 {
		for _, mount := range inspect.Mounts {
			source := mount.Name
			if source == "" {
				source = mount.Source
			}
			mode := mount.Mode
			if mode == "" {
				if mount.RW {
					mode = "rw"
				} else {
					mode = "ro"
				}
			}
			value := mount.Type + " · " + source + " · " + mount.Destination + " · " + mode
			serviceFact(c, p, "Mount", value, true)
		}
		return
	}
	if len(container.Mounts) > 0 {
		serviceFact(c, p, "Mounts", strings.Join(container.Mounts, ", "), true)
		return
	}
	ui.Text(c, "No mounts").FontSize(11).TextColor(p.TextMuted)
}

func (m *Model) dockerContainerNetworksView(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer,
) {
	inspect := m.dockerContainerInspect(node.dockerConn, container.ID)
	if inspect != nil && len(inspect.NetworkSettings.Networks) > 0 {
		names := make([]string, 0, len(inspect.NetworkSettings.Networks))
		for name := range inspect.NetworkSettings.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		ui.Text(c, "Networks").FontSize(12).Bold().TextColor(p.Text)
		for _, name := range names {
			network := inspect.NetworkSettings.Networks[name]
			parts := make([]string, 0, 4)
			if network.IPAddress != "" {
				parts = append(parts, network.IPAddress)
			}
			if network.GlobalIPv6Address != "" {
				parts = append(parts, network.GlobalIPv6Address)
			}
			if network.Gateway != "" {
				parts = append(parts, "gw "+network.Gateway)
			}
			if network.MacAddress != "" {
				parts = append(parts, network.MacAddress)
			}
			serviceFact(c, p, name, strings.Join(parts, " · "), true)
		}
		return
	}
	if len(container.Networks) > 0 {
		ui.Text(c, "Networks").FontSize(12).Bold().TextColor(p.Text)
		for _, name := range container.Networks {
			serviceFact(c, p, name, "", true)
		}
	}
}

func (m *Model) dockerContainerEnvironmentView(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, container openidea.DockerContainer,
) {
	ui.Text(c, "Environment Variables").FontSize(12).Bold().TextColor(p.Text)
	var env [][2]string
	if inspect := m.dockerContainerInspect(node.dockerConn, container.ID); inspect != nil {
		env = dockerEnvRows(inspect.Config.Env)
	}
	if len(env) == 0 {
		ui.Text(c, "No environment variables").FontSize(11).TextColor(p.TextMuted)
		return
	}
	for _, row := range env {
		serviceFact(c, p, row[0], row[1], true)
	}
}

func (m *Model) dockerImageInspectDetails(
	c *ui.Context, p *ui.Theme, node serviceTreeNode, image openidea.DockerImage,
) {
	inspect := m.dockerImageInspect(node.dockerConn, image.ID)
	if inspect == nil {
		return
	}
	ui.Text(c, "Image Details").FontSize(12).Bold().TextColor(p.Text)
	if platform := strings.Trim(strings.Join([]string{inspect.OS, inspect.Architecture}, "/"), "/"); platform != "" {
		serviceFact(c, p, "Platform", platform, false)
	}
	if size := dockerBytes(inspect.Size); size != "" {
		serviceFact(c, p, "Size (exact)", size, false)
	}
	if command := dockerInspectCommand(inspect.Config.Entrypoint, inspect.Config.Cmd, ""); command != "" {
		serviceFact(c, p, "Command", command, true)
	}
	if inspect.Config.User != "" {
		serviceFact(c, p, "User", inspect.Config.User, true)
	}
	if inspect.Config.WorkingDir != "" {
		serviceFact(c, p, "Working directory", inspect.Config.WorkingDir, true)
	}
	if ports := dockerExposedPorts(inspect.Config.ExposedPorts); len(ports) > 0 {
		serviceFact(c, p, "Exposed ports", strings.Join(ports, ", "), true)
	}
	if env := dockerEnvRows(inspect.Config.Env); len(env) > 0 {
		ui.Text(c, "Environment Variables").FontSize(12).Bold().TextColor(p.Text)
		for _, row := range env {
			serviceFact(c, p, row[0], row[1], true)
		}
	}
	m.dockerLabelsView(c, p, "Labels", inspect.Config.Labels)
}
