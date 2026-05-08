package models

import "time"

type VideoStatus string

const (
	StatusPending     VideoStatus = "pending"
	StatusUploading   VideoStatus = "uploading"
	StatusTranscoding VideoStatus = "transcoding"
	StatusReady       VideoStatus = "ready"
	StatusFailed      VideoStatus = "failed"
)

type Video struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Filename    string      `json:"filename"`
	Status      VideoStatus `json:"status"`
	Resolutions []string    `json:"resolutions"`
	Duration    float64     `json:"duration"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
