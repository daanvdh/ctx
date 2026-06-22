package model

import "time"

type ContextFile struct {
	Sessions map[string]*Session `json:"sessions"`
}

type Session struct {
	Parent  *string           `json:"parent"`
	Created time.Time         `json:"created"`
	Data    map[string]string `json:"data"`
}

type SessionNode struct {
	ID     string            `json:"id"`
	Parent *string           `json:"parent,omitempty"`
	Data   map[string]string `json:"data,omitempty"`
}
