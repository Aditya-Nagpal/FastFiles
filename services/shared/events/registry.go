package events

import "fmt"

type EventSchema int

const (
	SchemaEntry EventSchema = iota + 1
	SchemaS3Cleanup
)

var eventSchemas = map[string]EventSchema{
	EventFileCreated:    SchemaEntry,
	EventFileTrashed:    SchemaEntry,
	EventFolderTrashed:  SchemaEntry,
	EventFileRestored:   SchemaEntry,
	EventFolderRestored: SchemaEntry,

	EventS3Cleanup: SchemaS3Cleanup,
}

func GetEventSchema(eventType string) (EventSchema, error) {
	schema, exists := eventSchemas[eventType]
	if !exists {
		return 0, fmt.Errorf("unknown event type: %s", eventType)
	}

	return schema, nil
}
