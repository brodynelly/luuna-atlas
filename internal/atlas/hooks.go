package atlas

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type HookExecution struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exitCode"`
	Stderr     string `json:"stderr"`
	Stdout     string `json:"stdout"`
	DurationMS int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
}
type HookResult struct {
	Payload     any
	Blocked     bool
	BlockReason string
	Warnings    []string
	RanAny      bool
	Executions  []HookExecution
}

var hookEvents = []string{"pre-record", "post-record", "pre-prime", "pre-prune", "pre-compact"}

func runOneHook(cwd, command string, payload any, timeout time.Duration) HookExecution {
	start := time.Now()
	stdin, _ := MarshalJSON(payload)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "LUUNA_ATLAS_HOOK=1")
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	configureHookProcess(cmd)
	cmd.WaitDelay = 100 * time.Millisecond
	result := HookExecution{Command: command}
	if e := cmd.Start(); e != nil {
		result.ExitCode = 1
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case e := <-done:
		if e != nil {
			result.ExitCode = 1
			if cmd.ProcessState != nil {
				result.ExitCode = cmd.ProcessState.ExitCode()
			}
		}
	case <-timer.C:
		result.TimedOut = true
		result.ExitCode = 124
		killHookProcess(cmd)
		<-done
	}
	result.Stdout = out.String()
	result.Stderr = errOut.String()
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}
func hookDiagnostic(stderr io.Writer, event string, execution HookExecution, reason string, blocking bool, debug bool) {
	if stderr == nil {
		return
	}
	level, specified := os.LookupEnv("LUUNA_ATLAS_LOG_LEVEL")
	if !specified {
		level = "info"
		if os.Getenv("LUUNA_ATLAS_DEBUG") != "" {
			level = "debug"
		}
	}
	levels := map[string]int{"trace": 10, "debug": 20, "info": 30, "warn": 40, "error": 50, "fatal": 60, "silent": 100}
	messageLevel := 40
	if debug {
		messageLevel = 20
	}
	if levels[level] > messageLevel {
		return
	}
	host, _ := os.Hostname()
	m := map[string]any{"level": messageLevel, "time": time.Now().UnixMilli(), "pid": os.Getpid(), "hostname": host, "name": "luuna-atlas", "event": event, "command": execution.Command, "exitCode": execution.ExitCode, "timedOut": execution.TimedOut, "msg": reason}
	if debug {
		m["durationMs"] = execution.DurationMS
	} else {
		m["blocking"] = blocking
	}
	rememberOrder(m, []string{"level", "time", "pid", "hostname", "name", "event", "command", "exitCode", "durationMs", "timedOut", "blocking", "msg"})
	b, _ := MarshalJSON(m)
	fmt.Fprintln(stderr, string(b))
}
func RunHooks(cwd string, cfg Config, event string, payload any, stderr io.Writer) (HookResult, error) {
	result := HookResult{Payload: payload, Warnings: []string{}, Executions: []HookExecution{}}
	if !Contains(hookEvents, event) {
		return result, fmt.Errorf("Unknown hook event: %q.", event)
	}
	if cfg == nil {
		var err error
		cfg, err = ReadConfig(cwd)
		if err != nil {
			return result, nil
		}
	}
	scripts := List(Obj(cfg["hooks"])[event])
	timeout := Number(Obj(cfg["hook_settings"])["timeout_ms"])
	if timeout <= 0 {
		timeout = 5000
	}
	mutable := event == "pre-record" || event == "pre-prime" || event == "pre-compact"
	blocking := strings.HasPrefix(event, "pre-")
	for _, script := range scripts {
		if strings.TrimSpace(script) == "" {
			continue
		}
		envelope := map[string]any{"event": event, "payload": result.Payload}
		rememberOrder(envelope, []string{"event", "payload"})
		execution := runOneHook(cwd, script, envelope, time.Duration(timeout)*time.Millisecond)
		result.RanAny = true
		result.Executions = append(result.Executions, execution)
		hookDiagnostic(stderr, event, execution, "hook executed", blocking, true)
		if stderr != nil && execution.Stderr != "" {
			fmt.Fprint(stderr, execution.Stderr)
			if !strings.HasSuffix(execution.Stderr, "\n") {
				fmt.Fprintln(stderr)
			}
		}
		if execution.ExitCode != 0 {
			reason := fmt.Sprintf("hook `%s` exited with code %d", script, execution.ExitCode)
			if execution.TimedOut {
				reason = fmt.Sprintf("hook `%s` timed out after %gms", script, timeout)
			}
			hookDiagnostic(stderr, event, execution, reason, blocking, false)
			if blocking {
				result.Blocked = true
				result.BlockReason = reason
				return result, nil
			}
			result.Warnings = append(result.Warnings, reason)
			continue
		}
		if mutable && strings.TrimSpace(execution.Stdout) != "" {
			parsed, e := ParseJSON([]byte(strings.TrimSpace(execution.Stdout)))
			if e != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("hook `%s` printed non-JSON on stdout (mutation ignored): %s", script, writeJSONParseError(strings.TrimSpace(execution.Stdout))))
				continue
			}
			m := Obj(parsed)
			if value, ok := m["payload"]; ok {
				result.Payload = value
			} else {
				result.Payload = parsed
			}
		}
	}
	return result, nil
}
