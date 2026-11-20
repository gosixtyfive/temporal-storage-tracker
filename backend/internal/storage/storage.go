// Package storage holds the StorageItemWorkflow, its activities, and the types
// shared with the API. See docs/storage-workflow.md.
package storage

// TaskQueue is the Temporal task queue the worker polls and the API starts
// workflows on.
const TaskQueue = "storage-tracker"
