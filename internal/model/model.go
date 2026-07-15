package model

import "time"

type ContextFile struct {
	Sessions map[string]*Session `json:"sessions"`
}

type Session struct {
	Parent  *string           `json:"parent"`
	Created time.Time         `json:"created"`
	Data    map[string]string `json:"data"`
	Entries map[string]Entry  `json:"entries,omitempty"`
}

type SessionNode struct {
	ID      string            `json:"id"`
	Parent  *string           `json:"parent,omitempty"`
	Data    map[string]string `json:"data,omitempty"`
	Entries map[string]Entry  `json:"entries,omitempty"`
}

type ValueType string

const (
	ValueTypeString  ValueType = "string"
	ValueTypeFileRef ValueType = "file_ref"
	ValueTypeFileBin ValueType = "file_bin"
)

type Entry struct {
	Value     string    `json:"value,omitempty"`
	ValueType ValueType `json:"value_type"`
}

func NewEntry(value string, valueType ValueType) Entry {
	if valueType == "" {
		valueType = ValueTypeString
	}
	return Entry{Value: value, ValueType: valueType}
}
