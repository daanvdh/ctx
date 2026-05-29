package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"ctx/internal/model"
)

func GenID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func New(cf *model.ContextFile, parentID *string) (string, error) {
	id := GenID()

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
