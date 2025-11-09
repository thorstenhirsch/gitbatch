package git

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
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
	Source     eventQueueType
	Timestamp  time.Time
}

func (p tracedEventPayload) format() string {
	timestamp := p.Timestamp.UTC().Format(traceTimeFormat)
	indicator := queueIndicator(p.Source)
	summary := p.Summary
	if summary == "" {
		summary = "-"
	}
	if indicator != "" {
		indicator += " "
	}
	return fmt.Sprintf("%s %srepo=%s event=%s data=%s", timestamp, indicator, p.Repository, p.Event, summary)
}

func queueIndicator(source eventQueueType) string {
	switch source {
	case queueGit:
		return "[G]"
	case queueState:
		return "[S]"
	default:
		return ""
	}
}

func (r *Repository) traceEvent(eventName string, source eventQueueType, data interface{}) {
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
		Source:     source,
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
		if op := operationName(data); op != "" {
			return fmt.Sprintf("%T,operation=%s", data, op)
		}
		if payload, err := json.Marshal(v); err == nil {
			return string(payload)
		}
		return fmt.Sprintf("%T", data)
	}
}

func operationName(data interface{}) string {
	if data == nil {
		return ""
	}
	val := reflect.ValueOf(data)
	if !val.IsValid() {
		return ""
	}
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return ""
	}
	field := val.FieldByName("Operation")
	if !field.IsValid() {
		return ""
	}
	if !field.CanInterface() {
		return ""
	}
	value := field.Interface()
	if value == nil {
		return ""
	}
	name := strings.TrimSpace(fmt.Sprintf("%v", value))
	return name
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
