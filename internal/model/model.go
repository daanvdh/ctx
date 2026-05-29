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
