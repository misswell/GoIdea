package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mayoi/internal/openidea"
)

// A launch plan rewrites the run configuration and describes how to read the
// produced report. Round-trip both payloads so options unknown to Go stay put.
type coveragePlan struct {
	Status  string          `json:"status"`
	Launch  json.RawMessage `json:"launch"`
	Report  json.RawMessage `json:"report"`
	Reason  string          `json:"reason"`
	Tool    string          `json:"tool"`
	Message string          `json:"message"`
	Install *struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     *string  `json:"cwd"`
	} `json:"install"`
}

type coverageExecution struct {
	name, root string
	report     json.RawMessage
	stopped    bool
	early      []openidea.RunExitedEvent
}

// RunWithCoverage runs the selected configuration in the Run tool window.
// It uses the same save, trust and process lifecycle as a regular run.
func (m *Model) RunWithCoverage() {
	if m.Root() == "" || m.ctx == nil || m.busy || m.run.busy || m.run.running {
		return
	}
	m.run.pendingCoverage = true
	m.DockShow("run")
	if !m.run.trusted {
		m.run.pendingTrust = true
		m.trustAsk = true
		m.status = "Trust this project to run with coverage"
		m.invalidate()
		return
	}
	if !m.run.loaded {
		m.run.pendingStart = true
		m.loadRunConfigs()
		m.invalidate()
		return
	}
	m.startRun()
}

func (m *Model) prepareCoverage(ctx context.Context, config json.RawMessage) (coveragePlan, error) {
	var plan coveragePlan
	raw, err := m.invoke(ctx, "coverage_prepare", map[string]any{"config": config})
	if err != nil {
		return plan, err
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return plan, err
	}
	switch plan.Status {
	case "ready":
		if len(plan.Launch) == 0 || len(plan.Report) == 0 {
			return plan, fmt.Errorf("the coverage launch plan is incomplete")
		}
		return plan, nil
	case "missingTool":
		return plan, fmt.Errorf("%s is not installed", plan.Tool)
	case "unsupported":
		return plan, fmt.Errorf("%s", plan.Reason)
	default:
		return plan, fmt.Errorf("unknown coverage launch status %q", plan.Status)
	}
}

func (m *Model) reportCoveragePreparationError(name string, plan coveragePlan, err error) {
	if plan.Status != "missingTool" {
		m.coverageError("Cannot run "+name+" with coverage", err)
		return
	}
	var actions []notifyAction
	if install := plan.Install; install != nil {
		root := m.Root()
		actions = append(actions, notifyAction{label: "Install", run: func(m *Model) {
			if root != m.Root() {
				return
			}
			args := install.Args
			if args == nil {
				args = []string{}
			}
			cfg := openidea.RunConfig{ID: "install:" + plan.Tool, Name: "Install " + plan.Tool, Kind: "shell",
				Command: install.Command, Args: args, Cwd: install.Cwd, Env: map[string]string{}, Detected: true}
			raw, marshalErr := json.Marshal(cfg)
			if marshalErr != nil {
				m.coverageError(cfg.Name, marshalErr)
				return
			}
			m.startRunWith(raw, cfg.Name)
		}})
	}
	m.notifyWarning("run.coverage-tool", err.Error(), plan.Message, actions...)
}

func (e *coverageExecution) saveEarlyExit(exit openidea.RunExitedEvent) {
	const pendingExits = 8
	if len(e.early) == pendingExits {
		copy(e.early, e.early[1:])
		e.early = e.early[:pendingExits-1]
	}
	e.early = append(e.early, exit)
}

func (m *Model) collectCoverage(exec *coverageExecution, event openidea.RunExitedEvent) {
	if exec.stopped || event.Code == nil || m.Root() != exec.root {
		return
	}
	s := &m.coverage
	if s.projectToken == nil {
		s.projectToken = new(int)
	}
	token := s.projectToken
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		data, err := m.readCoverage(ctx, exec.root, "coverage_collect", map[string]any{"report": exec.report})
		m.post(func() {
			if m.Root() != exec.root || m.coverage.projectToken != token {
				return
			}
			if err != nil {
				m.coverageError("No coverage data for "+exec.name, err)
				return
			}
			m.coverage.err = ""
			suite := m.addCoverageSuite(exec.root, exec.name, "", data)
			m.DockShow("coverage")
			m.announceCoverage("Coverage for "+exec.name, suite)
		})
	}()
}
