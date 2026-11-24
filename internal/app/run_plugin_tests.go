package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

const (
	pluginTestChunkBytes  = 16_000
	pluginTestPendingMax  = 1 << 20
	pluginTestFallbackMax = 10_000
)

// runPluginTestOutput ports OpenIDEA's PluginTestOutput. A plugin-owned test
// configuration is parsed in bounded, sequential RPC batches. The built-in
// auto parser runs alongside it so a parser that is unavailable, invalid or
// too large falls back without losing the run.
type runPluginTestOutput struct {
	mu sync.Mutex

	m       *Model
	ctx     context.Context
	cancel  context.CancelFunc
	config  string
	root    string
	channel uint32
	tests   *adapter.TestResults

	mapper         *adapter.PluginTestMapper
	fallback       *adapter.AutoTestParser
	queued         string
	fallbackEvents []adapter.TestEvent
	pendingChars   int
	failed         bool
	stopping       bool
	wake           chan struct{}
	done           chan struct{}
}

func pluginTestConfigID(config *openidea.RunConfig) string {
	if config == nil {
		return ""
	}
	framework, _ := config.Params["framework"].(string)
	if framework != "plugin" {
		return ""
	}
	return strings.TrimSpace(config.ID)
}

func newRunPluginTestOutput(
	m *Model,
	config string,
	tests *adapter.TestResults,
) *runPluginTestOutput {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	output := &runPluginTestOutput{
		m:        m,
		ctx:      ctx,
		cancel:   cancel,
		config:   config,
		root:     m.Root(),
		channel:  m.run.channel,
		tests:    tests,
		mapper:   adapter.NewPluginTestMapper(),
		fallback: adapter.NewAutoTestParser(),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go output.watch()
	return output
}

func (p *runPluginTestOutput) Feed(line string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	fallbackEvents := p.fallback.Feed(line)
	p.fallbackEvents = append(p.fallbackEvents, fallbackEvents...)
	if len(p.fallbackEvents) > pluginTestFallbackMax {
		p.fallbackEvents = append(
			[]adapter.TestEvent(nil),
			p.fallbackEvents[len(p.fallbackEvents)-pluginTestFallbackMax:]...,
		)
	}
	if !p.failed && !p.stopping {
		chunk := line + "\n"
		p.queued += chunk
		p.pendingChars += len(chunk)
		if p.pendingChars > pluginTestPendingMax {
			p.failed = true
			p.queued = ""
			p.pendingChars = 0
		}
	}
	p.mu.Unlock()
	p.signal()
}

func (p *runPluginTestOutput) End(onDone func()) {
	if p == nil {
		if onDone != nil {
			onDone()
		}
		return
	}
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()
	p.signal()
	go func() {
		select {
		case <-p.done:
			if onDone != nil {
				p.m.post(func() {
					if p.current() {
						onDone()
					}
				})
			}
		case <-p.ctx.Done():
		}
	}()
}

func (p *runPluginTestOutput) stop() {
	if p != nil {
		p.cancel()
	}
}

func (p *runPluginTestOutput) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *runPluginTestOutput) watch() {
	defer close(p.done)
	for {
		if p.ctx.Err() != nil {
			return
		}
		p.mu.Lock()
		if p.failed {
			events := p.fallbackEvents
			p.fallbackEvents = nil
			stopping := p.stopping
			if stopping {
				events = append(events, p.fallback.End()...)
			}
			p.mu.Unlock()
			p.apply(events)
			if stopping {
				return
			}
			p.wait()
			continue
		}

		if p.queued != "" {
			chunk := p.takeChunkLocked()
			p.mu.Unlock()
			raw, err := p.m.invoke(p.ctx, "plugin_parse_test_results", map[string]any{
				"config": p.config,
				"chunk":  chunk,
			})
			if p.ctx.Err() != nil {
				return
			}
			if err != nil || isNullJSON(raw) {
				p.fail()
				continue
			}
			var pluginEvents []openidea.PluginTestEvent
			if err := json.Unmarshal(raw, &pluginEvents); err != nil {
				p.fail()
				continue
			}
			events, err := p.mapper.Map(pluginEvents)
			if err != nil {
				p.fail()
				continue
			}
			p.apply(events)
			continue
		}

		if p.stopping {
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		p.wait()
	}
}

func (p *runPluginTestOutput) takeChunkLocked() string {
	end := min(len(p.queued), pluginTestChunkBytes)
	for end > 0 && end < len(p.queued) && !utf8.RuneStart(p.queued[end]) {
		end--
	}
	chunk := p.queued[:end]
	p.queued = p.queued[end:]
	p.pendingChars -= len(chunk)
	return chunk
}

func (p *runPluginTestOutput) fail() {
	p.mu.Lock()
	p.failed = true
	p.queued = ""
	p.pendingChars = 0
	p.mu.Unlock()
	p.signal()
}

func (p *runPluginTestOutput) wait() {
	select {
	case <-p.ctx.Done():
	case <-p.wake:
	}
}

func (p *runPluginTestOutput) current() bool {
	return p != nil && p.m != nil &&
		p.m.Root() == p.root &&
		p.m.run.channel == p.channel &&
		p.m.run.tests == p.tests &&
		p.m.run.testPlugin == p
}

func (p *runPluginTestOutput) apply(events []adapter.TestEvent) {
	if len(events) == 0 {
		return
	}
	p.m.post(func() {
		if !p.current() {
			return
		}
		p.tests.Apply(events)
		p.m.invalidate()
	})
}

func isNullJSON(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func (m *Model) selectPluginFailedTests(
	base *openidea.RunConfig,
	configID string,
	paths [][]string,
) {
	root := m.Root()
	lastID := m.run.lastID
	m.run.rerunPluginGeneration++
	generation := m.run.rerunPluginGeneration
	m.status = "Selecting failed plugin tests…"
	m.invalidate()
	go func() {
		baseCtx := m.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
		defer cancel()
		raw, err := m.invoke(ctx, "plugin_select_tests", map[string]any{
			"config": configID,
			"tests":  paths,
		})
		var args []string
		if err == nil && !isNullJSON(raw) {
			err = json.Unmarshal(raw, &args)
		}
		m.post(func() {
			if m.Root() != root ||
				m.run.lastID != lastID ||
				m.run.rerunPluginGeneration != generation {
				return
			}
			if err != nil {
				if !isCancelledError(err) {
					m.notifyInfo(
						"run-rerun-failed",
						"No failed tests to rerun",
						"The plugin test selector failed: "+err.Error(),
					)
				}
				m.invalidate()
				return
			}
			if isNullJSON(raw) {
				m.notifyInfo(
					"run-rerun-failed",
					"No failed tests to rerun",
					"The configuration's plugin does not provide a test selector",
				)
				m.invalidate()
				return
			}
			derived := adapter.RerunConfigWithArgs(base, args)
			if derived == nil {
				m.notifyInfo(
					"run-rerun-failed",
					"No failed tests to rerun",
					"The plugin test selection could not be applied",
				)
				m.invalidate()
				return
			}
			m.askOrStart(derived.Raw(), derived.Name)
		})
	}()
}
