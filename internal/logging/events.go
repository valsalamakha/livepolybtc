package logging

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// EventType classifies a recorded event.
type EventType string

const (
	EventSignal    EventType = "SIGNAL"
	EventOrder     EventType = "ORDER"
	EventFill      EventType = "FILL"
	EventReject    EventType = "REJECT"
	EventPosition  EventType = "POSITION"
	EventSettle    EventType = "SETTLE"
	EventRisk      EventType = "RISK"
	EventHeartbeat EventType = "HEARTBEAT"
	EventError     EventType = "ERROR"
)

// Event is a single structured record written to the CSV and JSON event logs.
// Fields map directly to the columns required by the logging specification.
type Event struct {
	Timestamp      time.Time `json:"timestamp"`
	Type           EventType `json:"type"`
	Market         string    `json:"market"`
	BTCPrice       float64   `json:"btc_price"`
	BestBid        float64   `json:"best_bid"`
	BestAsk        float64   `json:"best_ask"`
	SignalStrength float64   `json:"signal_strength"`
	EntryReason    string    `json:"entry_reason"`
	OrderID        string    `json:"order_id"`
	Side           string    `json:"side"`
	Price          float64   `json:"price"`
	Size           float64   `json:"size"`
	FillPrice      float64   `json:"fill_price"`
	Profit         float64   `json:"profit"`
	LatencyMS      float64   `json:"latency_ms"`
	Message        string    `json:"message"`
}

var csvHeader = []string{
	"timestamp", "type", "market", "btc_price", "best_bid", "best_ask",
	"signal_strength", "entry_reason", "order_id", "side", "price", "size",
	"fill_price", "profit", "latency_ms", "message",
}

func (e Event) csvRow() []string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return []string{
		e.Timestamp.Format(time.RFC3339Nano),
		string(e.Type),
		e.Market,
		f(e.BTCPrice),
		f(e.BestBid),
		f(e.BestAsk),
		f(e.SignalStrength),
		e.EntryReason,
		e.OrderID,
		e.Side,
		f(e.Price),
		f(e.Size),
		f(e.FillPrice),
		f(e.Profit),
		f(e.LatencyMS),
		e.Message,
	}
}

// EventRecorder writes events to CSV and/or JSON-lines files. It is safe for
// concurrent use.
type EventRecorder struct {
	mu       sync.Mutex
	csvFile  *os.File
	csvW     *csv.Writer
	jsonFile *os.File
	closed   bool
}

// NewEventRecorder creates a recorder writing into dir. Timestamped file names
// keep runs separate. Either or both formats can be disabled.
func NewEventRecorder(dir string, enableCSV, enableJSON bool) (*EventRecorder, error) {
	if !enableCSV && !enableJSON {
		return &EventRecorder{}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	stamp := time.Now().Format("20060102-150405")
	r := &EventRecorder{}

	if enableCSV {
		path := filepath.Join(dir, "events-"+stamp+".csv")
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("create csv log: %w", err)
		}
		r.csvFile = f
		r.csvW = csv.NewWriter(f)
		if err := r.csvW.Write(csvHeader); err != nil {
			return nil, fmt.Errorf("write csv header: %w", err)
		}
		r.csvW.Flush()
	}

	if enableJSON {
		path := filepath.Join(dir, "events-"+stamp+".jsonl")
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("create json log: %w", err)
		}
		r.jsonFile = f
	}

	return r, nil
}

// Record writes a single event. The timestamp defaults to now when zero.
func (r *EventRecorder) Record(e Event) error {
	if r == nil {
		return nil
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}

	if r.csvW != nil {
		if err := r.csvW.Write(e.csvRow()); err != nil {
			return err
		}
		r.csvW.Flush()
		if err := r.csvW.Error(); err != nil {
			return err
		}
	}
	if r.jsonFile != nil {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		b = append(b, '\n')
		if _, err := r.jsonFile.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// Close flushes and closes the underlying files.
func (r *EventRecorder) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var firstErr error
	if r.csvW != nil {
		r.csvW.Flush()
	}
	if r.csvFile != nil {
		if err := r.csvFile.Close(); err != nil {
			firstErr = err
		}
	}
	if r.jsonFile != nil {
		if err := r.jsonFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
