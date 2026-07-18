// Package session generates and validates session IDs and keys.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"ctx/internal/model"
)

func GenID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ValidID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func ValidShellKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_' {
			continue
		}
		return false
	}
	return true
}

func New(cf *model.ContextFile, parentID *string) (string, error) {
	id, err := GenID()
	if err != nil {
		return "", err
	}

	if parentID != nil {
		if _, ok := cf.Sessions[*parentID]; !ok {
			return "", fmt.Errorf("parent session %s not found", *parentID)
		}
	}

	cf.Sessions[id] = &model.Session{
		Parent:  parentID,
		Created: time.Now(),
		Data:    make(map[string]string),
	}

	return id, nil
}

func Set(cf *model.ContextFile, sessionID, key, value string) error {
	s, ok := cf.Sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if s.Data == nil {
		s.Data = make(map[string]string)
	}
	s.Data[key] = value
	return nil
}

func Get(cf *model.ContextFile, sessionID, key string) (string, error) {
	if cf == nil || cf.Sessions == nil {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	if _, ok := cf.Sessions[sessionID]; !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	visited := make(map[string]bool)
	currentID := sessionID
	hops := 0

	for currentID != "" && hops < 50 {
		hops++
		s, ok := cf.Sessions[currentID]
		if !ok {
			break
		}
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		if val, ok := s.Data[key]; ok {
			return val, nil
		}

		currentID = parentID(s)
	}

	return "", fmt.Errorf("key %s not found in session %s or ancestors", key, sessionID)
}

func Resolve(cf *model.ContextFile, sessionID string) (map[string]string, error) {
	if cf == nil || cf.Sessions == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	if _, ok := cf.Sessions[sessionID]; !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	visited := make(map[string]bool)
	result := make(map[string]string)
	currentID := sessionID
	hops := 0

	for currentID != "" && hops < 50 {
		hops++
		s, ok := cf.Sessions[currentID]
		if !ok {
			break
		}
		if visited[currentID] {
			break
		}
		visited[currentID] = true

		for k, v := range s.Data {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}

		currentID = parentID(s)
	}

	return result, nil
}

func parentID(s *model.Session) string {
	if s.Parent == nil {
		return ""
	}
	return *s.Parent
}
