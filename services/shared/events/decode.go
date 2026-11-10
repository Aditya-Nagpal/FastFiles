package events

import (
	"encoding/json"
	"fmt"
)

func DecodeEvent(payload []byte) (any, error) {
	var header struct {
		EventType string `json:"eventType"`
	}

	if err := json.Unmarshal(payload, &header); err != nil {
		return nil, fmt.Errorf("decode event header: %w", err)
	}

	schema, err := GetEventSchema(header.EventType)
	if err != nil {
		return nil, err
	}

	switch schema {
	case SchemaEntry:
		var event EntryEvent

		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("decode entry event: %w", err)
		}

		return event, nil

	case SchemaS3Cleanup:
		var event S3CleanupEvent

		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("decode S3 cleanup event: %w", err)
		}

		return event, nil

	default:
		return nil, fmt.Errorf("unsupported event schema: %d", schema)
	}
}
