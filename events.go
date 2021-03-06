package nakadi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
)

// An EventType defines a kind of event that can be processed on a Nakadi service.
type EventType struct {
	Name                 string               `json:"name"`
	OwningApplication    string               `json:"owning_application"`
	Category             string               `json:"category"`
	EnrichmentStrategies []string             `json:"enrichment_strategies,omitempty"`
	PartitionStrategy    string               `json:"partition_strategy,omitempty"`
	CompatibilityMode    string               `json:"compatibility_mode,omitempty"`
	Schema               *EventTypeSchema     `json:"schema"`
	PartitionKeyFields   []string             `json:"partition_key_fields"`
	DefaultStatistics    *EventTypeStatistics `json:"default_statistics,omitempty"`
	Options              *EventTypeOptions    `json:"options,omitempty"`
	CreatedAt            time.Time            `json:"created_at,omitempty"`
	UpdatedAt            time.Time            `json:"updated_at,omitempty"`
}

// EventTypeSchema is a non optional description of the schema on an event type.
type EventTypeSchema struct {
	Version   string    `json:"version,omitempty"`
	Type      string    `json:"type"`
	Schema    string    `json:"schema"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// EventTypeStatistics describe operational statistics for an event type. This statistics are
// used by Nakadi to optimize the throughput events from a certain kind. They are provided on
// event type creation.
type EventTypeStatistics struct {
	MessagesPerMinute int `json:"messages_per_minute"`
	MessageSize       int `json:"message_size"`
	ReadParallelism   int `json:"read_parallelism"`
	WriteParallelism  int `json:"write_parallelism"`
}

// EventTypeOptions provide additional parameters for tuning Nakadi.
type EventTypeOptions struct {
	RetentionTime int64 `json:"retention_time"`
}

// EventOptions is a set of optional parameters used to configure the EventAPI.
type EventOptions struct {
	// Whether or not methods of the EventAPI retry when a request fails. If
	// set to true InitialRetryInterval, MaxRetryInterval, and MaxElapsedTime have
	// no effect (default: false).
	Retry bool
	// The initial (minimal) retry interval used for the exponential backoff algorithm
	// when retry is enables.
	InitialRetryInterval time.Duration
	// MaxRetryInterval the maximum retry interval. Once the exponential backoff reaches
	// this value the retry intervals remain constant.
	MaxRetryInterval time.Duration
	// MaxElapsedTime is the maximum time spent on retries when when performing a request.
	// Once this value was reached the exponential backoff is halted and the request will
	// fail with an error.
	MaxElapsedTime time.Duration
}

func (o *EventOptions) withDefaults() *EventOptions {
	var copyOptions EventOptions
	if o != nil {
		copyOptions = *o
	}
	if copyOptions.InitialRetryInterval == 0 {
		copyOptions.InitialRetryInterval = defaultInitialRetryInterval
	}
	if copyOptions.MaxRetryInterval == 0 {
		copyOptions.MaxRetryInterval = defaultMaxRetryInterval
	}
	if copyOptions.MaxElapsedTime == 0 {
		copyOptions.MaxElapsedTime = defaultMaxElapsedTime
	}
	return &copyOptions
}

// NewEventAPI creates a new instance of a EventAPI implementation which can be used to
// manage event types on a specific Nakadi service. The last parameter is a struct containing only
// optional parameters. The options may be nil.
func NewEventAPI(client *Client, options *EventOptions) *EventAPI {
	options = options.withDefaults()

	var backOff backoff.BackOff
	if options.Retry {
		back := backoff.NewExponentialBackOff()
		back.InitialInterval = options.InitialRetryInterval
		back.MaxInterval = options.MaxRetryInterval
		back.MaxElapsedTime = options.MaxElapsedTime
		backOff = back
	} else {
		backOff = &backoff.StopBackOff{}
	}
	return &EventAPI{
		client:  client,
		backOff: backOff}
}

// EventAPI is a sub API that allows to inspect and manage event types on a Nakadi instance.
type EventAPI struct {
	client  *Client
	backOff backoff.BackOff
}

// List returns all registered event types.
func (e *EventAPI) List() ([]*EventType, error) {
	eventTypes := []*EventType{}
	err := e.client.httpGET(e.backOff, e.eventBaseURL(), &eventTypes, "unable to request event types")
	if err != nil {
		return nil, err
	}
	return eventTypes, nil
}

// Get returns an event type based on its name.
func (e *EventAPI) Get(name string) (*EventType, error) {
	eventType := &EventType{}
	err := e.client.httpGET(e.backOff, e.eventURL(name), eventType, "unable to request event types")
	if err != nil {
		return nil, err
	}
	return eventType, nil
}

// Create saves a new event type.
func (e *EventAPI) Create(eventType *EventType) error {
	response, err := e.client.httpPOST(e.backOff, e.eventBaseURL(), eventType)
	if err != nil {
		return errors.Wrap(err, "unable to create event type")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		problem := problemJSON{}
		err := json.NewDecoder(response.Body).Decode(&problem)
		if err != nil {
			return errors.Wrap(err, "unable to decode response body")
		}
		return errors.Errorf("unable to create event type: %s", problem.Detail)
	}

	return nil
}

// Update updates an existing event type.
func (e *EventAPI) Update(eventType *EventType) error {
	response, err := e.client.httpPUT(e.backOff, e.eventURL(eventType.Name), eventType)
	if err != nil {
		return errors.Wrap(err, "unable to update event type")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		problem := problemJSON{}
		err := json.NewDecoder(response.Body).Decode(&problem)
		if err != nil {
			return errors.Wrap(err, "unable to decode response body")
		}
		return errors.Errorf("unable to update event type: %s", problem.Detail)
	}

	return nil
}

// Delete removes an event type.
func (e *EventAPI) Delete(name string) error {
	return e.client.httpDELETE(e.backOff, e.eventURL(name), "unable to delete event type")
}

func (e *EventAPI) eventURL(name string) string {
	return fmt.Sprintf("%s/event-types/%s", e.client.nakadiURL, name)
}

func (e *EventAPI) eventBaseURL() string {
	return fmt.Sprintf("%s/event-types", e.client.nakadiURL)
}
