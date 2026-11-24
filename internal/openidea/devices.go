package openidea

import "encoding/json"

// AndroidDevice is one entry from `adb devices -l`.
type AndroidDevice struct {
	Serial      string  `json:"serial"`
	State       string  `json:"state"`
	Model       *string `json:"model"`
	Product     *string `json:"product"`
	Device      *string `json:"device"`
	TransportID *string `json:"transportId"`
}

// LogcatLine is one line from `adb logcat -v threadtime`.
type LogcatLine struct {
	Time    string `json:"time"`
	PID     int    `json:"pid"`
	TID     int    `json:"tid"`
	Level   string `json:"level"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

// ProfileMode selects what Run with Profiler measures.
type ProfileMode string

const (
	ProfileCPU         ProfileMode = "cpu"
	ProfileAllocations ProfileMode = "allocations"
	ProfileLiveObjects ProfileMode = "liveObjects"
)

// ProfilerInstall is a command that installs a missing profiler.
type ProfilerInstall struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Cwd     *string  `json:"cwd"`
}

// ProfilePrepared is the backend's ready/missing-tool/unsupported plan.
type ProfilePrepared struct {
	Status   string           `json:"status"`
	Profiler string           `json:"profiler"`
	Launch   *RunConfig       `json:"-"`
	Spec     json.RawMessage  `json:"spec"`
	Tool     string           `json:"tool"`
	Message  string           `json:"message"`
	Install  *ProfilerInstall `json:"install"`
	Homepage *string          `json:"homepage"`
	Reason   string           `json:"reason"`
}

// UnmarshalJSON keeps the ready launch configuration's complete raw object.
// RunConfig intentionally hides its private raw field from encoding/json, so
// decoding it directly here would lose fields that profiler_prepare wraps.
func (p *ProfilePrepared) UnmarshalJSON(data []byte) error {
	var wire struct {
		Status   string           `json:"status"`
		Profiler string           `json:"profiler"`
		Launch   json.RawMessage  `json:"launch"`
		Spec     json.RawMessage  `json:"spec"`
		Tool     string           `json:"tool"`
		Message  string           `json:"message"`
		Install  *ProfilerInstall `json:"install"`
		Homepage *string          `json:"homepage"`
		Reason   string           `json:"reason"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	p.Status = wire.Status
	p.Profiler = wire.Profiler
	p.Spec = append(json.RawMessage(nil), wire.Spec...)
	p.Tool = wire.Tool
	p.Message = wire.Message
	p.Install = wire.Install
	p.Homepage = wire.Homepage
	p.Reason = wire.Reason
	p.Launch = nil
	if len(wire.Launch) > 0 && string(wire.Launch) != "null" {
		launch, err := DecodeRunConfig(wire.Launch)
		if err != nil {
			return err
		}
		p.Launch = launch
	}
	return nil
}

// Profile is a sampled call tree, its frames and rolled-up methods.
type Profile struct {
	Profiler string         `json:"profiler"`
	Unit     string         `json:"unit"`
	Mode     ProfileMode    `json:"mode"`
	Total    *float64       `json:"total"`
	Frames   []ProfileFrame `json:"frames"`
	Nodes    []ProfileNode  `json:"nodes"`
	Methods  []MethodStat   `json:"methods"`
}

// ProfileFrame is one code location or allocated type.
type ProfileFrame struct {
	Name string  `json:"name"`
	File *string `json:"file"`
	Line *int    `json:"line"`
	Kind string  `json:"kind"`
}

// ProfileNode is one node in the depth-first call tree.
type ProfileNode struct {
	Frame      int      `json:"frame"`
	Total      *float64 `json:"total"`
	SelfWeight *float64 `json:"selfWeight"`
	Children   []int    `json:"children"`
}

// MethodStat is one method's self and inclusive weight.
type MethodStat struct {
	Frame      int      `json:"frame"`
	SelfWeight *float64 `json:"selfWeight"`
	Total      *float64 `json:"total"`
}
