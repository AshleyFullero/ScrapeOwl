package runner

import (
	"time"
)

// EventType represents the type of a run event
type EventType string

const (
	EventTypeLog      EventType = "log"
	EventTypeStep     EventType = "step"
	EventTypeExtract  EventType = "extract"
	EventTypeOutput   EventType = "output"
	EventTypeStatus   EventType = "status"
	EventTypeError    EventType = "error"
	EventTypeComplete EventType = "complete"
)

// LogLevel represents the severity of a log event
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Event is a real-time event emitted during job execution
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Message   string      `json:"message,omitempty"`
	Level     LogLevel    `json:"level,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	JobName   string      `json:"job_name,omitempty"`
	RunID     string      `json:"run_id,omitempty"`
}

// StatusEvent is the data for a status change event
type StatusEvent struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"` // 0-100
}

// StepEvent is the data for a step execution event
type StepEvent struct {
	Index    int    `json:"index"`
	Action   string `json:"action"`
	Selector string `json:"selector,omitempty"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ExtractEvent is the data for a data extraction event
type ExtractEvent struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// EventBus is a simple pub/sub event bus for run events
type EventBus struct {
	listeners []chan Event
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe adds a listener channel and returns it
func (b *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 100)
	b.listeners = append(b.listeners, ch)
	return ch
}

// Publish sends an event to all listeners (non-blocking)
func (b *EventBus) Publish(e Event) {
	for _, ch := range b.listeners {
		select {
		case ch <- e:
		default:
			// Listener is full, drop event to avoid blocking
		}
	}
}

// Close closes all listener channels
func (b *EventBus) Close() {
	for _, ch := range b.listeners {
		close(ch)
	}
	b.listeners = nil
}

// Log emits a log event
func (b *EventBus) Log(level LogLevel, msg string, jobName, runID string) {
	b.Publish(Event{
		Type:      EventTypeLog,
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
		JobName:   jobName,
		RunID:     runID,
	})
}

// SetStatus emits a status change event
func (b *EventBus) SetStatus(status string, progress int, jobName, runID string) {
	b.Publish(Event{
		Type:      EventTypeStatus,
		Timestamp: time.Now(),
		JobName:   jobName,
		RunID:     runID,
		Data:      StatusEvent{Status: status, Progress: progress},
	})
}
