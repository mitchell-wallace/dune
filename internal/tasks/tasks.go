package tasks

import "time"

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
)

type EventType string

const (
	EventConfigResolved            EventType = "config_resolved"
	EventContainerProvisionStarted EventType = "container_provision_started"
	EventContainerProvisionEnded   EventType = "container_provision_finished"
	EventAddonInstallStarted       EventType = "addon_install_started"
	EventAddonInstallEnded         EventType = "addon_install_finished"
	EventShellAttached             EventType = "shell_attached"
)

type Task struct {
	Name      string
	Status    TaskStatus
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
}

type Event struct {
	Type      EventType
	Message   string
	Timestamp time.Time
}
