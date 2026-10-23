package events

//go:generate go run ./internal/catalogen/cmd ../../../../docs/architecture/events.yaml catalog.gen.go

// Spec is one catalog entry: which module may publish the event and which fields its data carries.
type Spec struct {
	Producer string
	Data     []string
}
