package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

func dockerImageTagsForID(snapshot openidea.DockerSnapshot, image openidea.DockerImage) []string {
	seen := map[string]bool{}
	tags := make([]string, 0)
	for _, candidate := range snapshot.Images {
		if candidate.ID != image.ID || dockerUntaggedImage(candidate) {
			continue
		}
		ref := dockerImageRef(candidate)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		tags = append(tags, ref)
	}
	sort.Strings(tags)
	return tags
}

func mergeDockerImageTags(left, right []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(left)+len(right))
	for _, tags := range [][]string{left, right} {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

func dockerContainerMatchesImage(container openidea.DockerContainer, image openidea.DockerImage) bool {
	ref := dockerImageRef(image)
	if container.Image == ref || container.Image == image.ID || container.Image == dockerShortID(image.ID) {
		return true
	}
	if image.Tag == "latest" && container.Image == image.Repository {
		return true
	}
	cleanID := strings.TrimPrefix(image.ID, "sha256:")
	cleanTarget := strings.TrimPrefix(container.Image, "sha256:")
	return len(cleanTarget) >= 12 && strings.HasPrefix(cleanID, cleanTarget)
}

func dockerContainersUsingImage(
	snapshot openidea.DockerSnapshot,
	image openidea.DockerImage,
) []openidea.DockerContainer {
	out := make([]openidea.DockerContainer, 0)
	for _, container := range snapshot.Containers {
		if dockerContainerMatchesImage(container, image) {
			out = append(out, container)
		}
	}
	return out
}

func dockerContainersUsingVolume(
	snapshot openidea.DockerSnapshot,
	volume openidea.DockerVolume,
) []openidea.DockerContainer {
	out := make([]openidea.DockerContainer, 0)
	for _, container := range snapshot.Containers {
		for _, mount := range container.Mounts {
			if mount == volume.Name {
				out = append(out, container)
				break
			}
		}
	}
	return out
}

func dockerContainersInNetwork(
	snapshot openidea.DockerSnapshot,
	network openidea.DockerNetwork,
) []openidea.DockerContainer {
	out := make([]openidea.DockerContainer, 0)
	for _, container := range snapshot.Containers {
		for _, connected := range container.Networks {
			if connected == network.Name {
				out = append(out, container)
				break
			}
		}
	}
	return out
}

func dockerContainerNames(containers []openidea.DockerContainer) string {
	names := make([]string, 0, len(containers))
	for _, container := range containers {
		name := container.Name
		if name == "" {
			name = dockerShortID(container.ID)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func boolText(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

type dockerPortMapping struct {
	hostIP        string
	hostPort      string
	containerPort string
	protocol      string
	url           string
	label         string
}

func dockerPortMappings(text string) []dockerPortMapping {
	seen := map[string]bool{}
	out := make([]dockerPortMapping, 0)
	push := func(rawHostIP, hostPort, containerPort, protocol string) {
		hostIP := rawHostIP
		if hostIP == "::" {
			hostIP = "[::]"
		}
		key := fmt.Sprintf("%s:%s->%s/%s", hostIP, hostPort, containerPort, protocol)
		if seen[key] {
			return
		}
		seen[key] = true
		browseHost := hostIP
		if browseHost == "" || browseHost == "0.0.0.0" || browseHost == "[::]" {
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

	for _, segment := range strings.Split(text, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		at := strings.Index(segment, "->")
		if at < 0 {
			containerPort, protocol, _ := strings.Cut(segment, "/")
			if protocol == "" {
				protocol = "tcp"
			}
			push("", "", containerPort, protocol)
			continue
		}
		left, right := segment[:at], segment[at+2:]
		containerPort, protocol, _ := strings.Cut(right, "/")
		if protocol == "" {
			protocol = "tcp"
		}
		colon := strings.LastIndexByte(left, ':')
		hostIP, hostPort := "0.0.0.0", left
		if colon >= 0 {
			hostIP, hostPort = left[:colon], left[colon+1:]
		}
		push(hostIP, hostPort, containerPort, protocol)
	}
	return out
}

func (m *Model) dockerPortBindings(c *ui.Context, p *ui.Theme, ports string) {
	mappings := dockerPortMappings(ports)
	ui.Text(c, "Port Bindings").FontSize(12).Bold().TextColor(p.Text)
	if len(mappings) == 0 {
		ui.Text(c, "No port bindings").FontSize(11).TextColor(p.TextMuted)
		return
	}
	m.dockerPortMappingRows(c, p, mappings)
}

func (m *Model) dockerPortMappingRows(
	c *ui.Context,
	p *ui.Theme,
	mappings []dockerPortMapping,
) {
	if len(mappings) == 0 {
		return
	}
	for _, mapping := range mappings {
		mapping := mapping
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			text := ui.Text(c, mapping.label).FontSize(11).Font("monospace").TextColor(p.TextMuted)
			if mapping.url != "" {
				text.TextColor(p.Accent).Cursor(ui.CursorPointer).OnClick(func() { c.OpenURL(mapping.url) })
			}
		})
	}
}

func dockerCopyableFact(
	c *ui.Context,
	p *ui.Theme,
	label, value, copied string,
	monospace bool,
) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.TextMuted).Width(92)
		text := ui.Text(c, value).FontSize(12).TextColor(p.Text).Grow(1)
		if monospace {
			text.Font("monospace")
		}
		ui.Button(c, "Copy").OnClick(func() { mygo.Clipboard.WriteText(copied) })
	})
}

func (m *Model) dockerLabelsView(
	c *ui.Context,
	p *ui.Theme,
	title string,
	labels map[string]string,
) {
	if len(labels) == 0 {
		return
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ui.Text(c, title).FontSize(12).Bold().TextColor(p.Text)
	for _, key := range keys {
		serviceFact(c, p, key, labels[key], true)
	}
}
