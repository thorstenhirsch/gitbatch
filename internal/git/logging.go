package git

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	traceLogFileName  = "gitbatch.log"
	traceEventMaxData = 512
	traceTimeFormat   = "2006-01-02T15:04:05.000000"
)

type traceSettings struct {
	enabled bool
	path    string
	logger  *eventTraceLogger
}

var (
	traceSettingsMu sync.RWMutex
	currentTrace    traceSettings
)

type eventTraceLogger struct {
	mu   sync.Mutex
	file *os.File
}

func (l *eventTraceLogger) write(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return fmt.Errorf("trace logger not initialized")
	}
	_, err := l.file.WriteString(line + "\n")
	return err
}

func (l *eventTraceLogger) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// SetTraceLogging enables or disables trace logging across repositories.
// When enabled a gitbatch.log file is created in the current working directory.
func SetTraceLogging(enabled bool) error {
	traceSettingsMu.Lock()
	defer traceSettingsMu.Unlock()

	if !enabled {
		if currentTrace.logger != nil {
			_ = currentTrace.logger.close()
		}
		currentTrace = traceSettings{}
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	path := filepath.Join(wd, traceLogFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	if currentTrace.logger != nil {
		_ = currentTrace.logger.close()
	}

	currentTrace = traceSettings{
		enabled: true,
		path:    path,
		logger:  &eventTraceLogger{file: file},
	}

	return nil
}

func isTraceEnabled() bool {
	traceSettingsMu.RLock()
	enabled := currentTrace.enabled && currentTrace.logger != nil
	traceSettingsMu.RUnlock()
	return enabled
}

type tracedEventPayload struct {
	Repository string
	Event      string
	Summary    string
	Timestamp  time.Time
}

func (p tracedEventPayload) format() string {
	timestamp := p.Timestamp.UTC().Format(traceTimeFormat)
	summary := p.Summary
	if summary == "" {
		summary = "-"
	}
	return fmt.Sprintf("%s repo=%s event=%s data=%s", timestamp, p.Repository, p.Event, summary)
}

func (r *Repository) traceEvent(eventName string, data interface{}) {
	if r == nil || !isTraceEnabled() || eventName == RepositoryEventTraced {
		return
	}
	logQueue := r.queues[queueLog]
	if logQueue == nil {
		return
	}

	summary := sanitizeTraceValue(renderTraceData(data))
	payload := tracedEventPayload{
		Repository: r.Name,
		Event:      eventName,
		Summary:    summary,
		Timestamp:  time.Now().UTC(),
	}

	logEvent := &RepositoryEvent{Name: RepositoryEventTraced, Data: payload}
	if err := logQueue.enqueue(logEvent); err != nil {
		log.Printf("log queue enqueue failed: %v", err)
	}
}

func renderTraceData(data interface{}) string {
	switch v := data.(type) {
	case nil:
		return "nil"
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case error:
		return v.Error()
	default:
		if payload, err := json.Marshal(v); err == nil {
			return string(payload)
		}
		return fmt.Sprintf("%T", data)
	}
}

func sanitizeTraceValue(value string) string {
	if len(value) == 0 {
		return ""
	}
	replaced := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(value)
	replaced = strings.TrimSpace(replaced)
	if len(replaced) > traceEventMaxData {
		replaced = replaced[:traceEventMaxData] + "..."
	}
	return replaced
}

func writeTraceLine(payload tracedEventPayload) {
	traceSettingsMu.RLock()
	logger := currentTrace.logger
	traceSettingsMu.RUnlock()
	if logger == nil {
		return
	}
	if err := logger.write(payload.format()); err != nil {
		log.Printf("trace log write failed: %v", err)
	}
}

func traceLogListener(event *RepositoryEvent) error {
	payload, ok := event.Data.(tracedEventPayload)
	if !ok {
		return nil
	}
	writeTraceLine(payload)
	return nil
}

func attachTraceLogger(r *Repository) {
	if r == nil || !isTraceEnabled() {
		return
	}
	r.On(RepositoryEventTraced, traceLogListener)
}

func init() {
	RegisterRepositoryHook(attachTraceLogger)
}
