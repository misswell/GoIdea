package openidea

// DockerEndpoint selects the daemon a Docker command talks to.
type DockerEndpoint struct {
	Kind       string  `json:"kind"`
	Path       string  `json:"path,omitempty"`
	Host       string  `json:"host,omitempty"`
	TLSCertDir *string `json:"tlsCertDir,omitempty"`
	Name       string  `json:"name,omitempty"`
}

// DockerAvailability says whether Docker can be used and why not.
type DockerAvailability struct {
	Kind          string  `json:"kind"`
	ClientVersion string  `json:"clientVersion,omitempty"`
	ServerVersion string  `json:"serverVersion,omitempty"`
	Context       *string `json:"context,omitempty"`
	Message       string  `json:"message,omitempty"`
}

// DockerConnection is one user-added Docker daemon in Services.
type DockerConnection struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Endpoint DockerEndpoint `json:"endpoint"`
}

// DockerRegistry is one registry login target.
type DockerRegistry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Username string `json:"username,omitempty"`
}

// DockerSettings are the user's Services Docker connections and registries.
type DockerSettings struct {
	Connections []DockerConnection `json:"connections"`
	Registries  []DockerRegistry   `json:"registries"`
}

// DockerContainer is one container row from docker ps.
type DockerContainer struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Image             string            `json:"image"`
	Command           string            `json:"command"`
	State             string            `json:"state"`
	Status            string            `json:"status"`
	Ports             string            `json:"ports"`
	CreatedAt         string            `json:"createdAt"`
	ComposeProject    *string           `json:"composeProject"`
	ComposeService    *string           `json:"composeService"`
	ComposeWorkingDir *string           `json:"composeWorkingDir"`
	ComposeFiles      []string          `json:"composeFiles"`
	Mounts            []string          `json:"mounts,omitempty"`
	Networks          []string          `json:"networks,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
}

// DockerImage is one local image tag.
type DockerImage struct {
	ID           string `json:"id"`
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
	Size         string `json:"size"`
	CreatedSince string `json:"createdSince"`
}

// DockerVolume is one local volume.
type DockerVolume struct {
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	Mountpoint     string            `json:"mountpoint"`
	ComposeProject *string           `json:"composeProject"`
	Scope          string            `json:"scope,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// DockerNetwork is one local network.
type DockerNetwork struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	Scope          string            `json:"scope"`
	ComposeProject *string           `json:"composeProject"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	IPv6           bool              `json:"ipv6,omitempty"`
	Internal       bool              `json:"internal,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// DockerSnapshot is everything the Services Docker provider shows.
type DockerSnapshot struct {
	Availability DockerAvailability `json:"availability"`
	Executable   *string            `json:"executable"`
	Containers   []DockerContainer  `json:"containers"`
	Images       []DockerImage      `json:"images"`
	Volumes      []DockerVolume     `json:"volumes"`
	Networks     []DockerNetwork    `json:"networks"`
	Errors       []string           `json:"errors"`
	GlobalArgs   []string           `json:"globalArgs,omitempty"`
}

// DockerContainerStats is one `docker stats --no-stream` row.
type DockerContainerStats struct {
	CPUPerc  string `json:"cpuPerc"`
	MemUsage string `json:"memUsage"`
	MemPerc  string `json:"memPerc"`
	NetIO    string `json:"netIo"`
	BlockIO  string `json:"blockIo"`
	Pids     string `json:"pids"`
}

// DockerImageLayer is one image history row.
type DockerImageLayer struct {
	ID           string `json:"id"`
	CreatedBy    string `json:"createdBy"`
	Size         string `json:"size"`
	CreatedSince string `json:"createdSince"`
	CreatedAt    string `json:"createdAt"`
	Comment      string `json:"comment"`
}

// DockerProcessList is the reply to docker_container_top.
type DockerProcessList struct {
	Titles    []string   `json:"titles"`
	Processes [][]string `json:"processes"`
}

// DockerEvent is one relevant line from docker events.
type DockerEvent struct {
	Object string  `json:"object"`
	Action string  `json:"action"`
	ID     string  `json:"id"`
	Name   *string `json:"name"`
}

// DockerLogLine is one line streamed by docker_logs.
type DockerLogLine struct {
	Stderr bool   `json:"stderr"`
	Text   string `json:"text"`
}
