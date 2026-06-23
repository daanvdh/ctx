package render

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"ctx/internal/model"
)

const (
	connBranch = "\u251c\u2500\u2500 "
	connLast   = "\u2514\u2500\u2500 "
	cont       = "\u2502   "
	space      = "    "
)

func Tree(cf *model.ContextFile) (string, error) {
	if cf == nil || cf.Sessions == nil {
		return "", nil
	}

	children := map[string][]string{}
	roots := []string{}

	for id, s := range cf.Sessions {
		if s.Parent == nil || *s.Parent == "" {
			roots = append(roots, id)
		} else {
			children[*s.Parent] = append(children[*s.Parent], id)
		}
	}

	sort.Strings(roots)
	for k := range children {
		sort.Strings(children[k])
	}

	var sb strings.Builder
	for i, r := range roots {
		isLastChild := i == len(roots)-1
		writeNode(&sb, cf, r, children, "", true, isLastChild)
	}

	return sb.String(), nil
}

func TreeNodes(nodes []model.SessionNode) (string, error) {
	if len(nodes) == 0 {
		return "", nil
	}

	cf := &model.ContextFile{Sessions: make(map[string]*model.Session, len(nodes))}
	for _, node := range nodes {
		cf.Sessions[node.ID] = &model.Session{
			Parent:  node.Parent,
			Data:    node.Data,
			Entries: node.Entries,
		}
	}
	return Tree(cf)
}

func writeNode(sb *strings.Builder, cf *model.ContextFile, id string, children map[string][]string, prefix string, isRoot, isLastInParent bool) {
	s := cf.Sessions[id]

	if isRoot {
		sb.WriteString(id)
	} else if isLastInParent {
		sb.WriteString(prefix + connLast + id)
	} else {
		sb.WriteString(prefix + connBranch + id)
	}
	sb.WriteString("\n")

	keys := sortKeys(s.Data)
	for _, k := range keys {
		var indent string
		if isRoot {
			indent = " "
		} else {
			indent = strings.Repeat(" ", 4+len(prefix))
		}
		entry := model.NewEntry(s.Data[k], model.ValueTypeString)
		if s.Entries != nil {
			if typed, ok := s.Entries[k]; ok {
				entry = typed
			}
		}
		sb.WriteString(indent + formatEntry(k, entry) + "\n")
	}

	if kids, ok := children[id]; ok {
		for j, childID := range kids {
			childIsLast := j == len(kids)-1

			var newPrefix string
			if isRoot {
				newPrefix = ""
			} else if !isLastInParent {
				newPrefix = prefix + cont
			} else {
				newPrefix = prefix + space
			}

			writeNode(sb, cf, childID, children, newPrefix, false, childIsLast)
		}
	}
}

func formatEntry(key string, entry model.Entry) string {
	switch entry.ValueType {
	case model.ValueTypeDoc:
		return fmt.Sprintf("%s [doc] %s %q", key, humanKB(len([]byte(entry.Value))), preview(entry.Value, 60))
	case model.ValueTypeFileRef:
		if _, err := os.Stat(entry.Value); err != nil && os.IsNotExist(err) {
			return fmt.Sprintf("%s [file_ref] %s", key, "\u26a0 path not found")
		}
		return fmt.Sprintf("%s [file_ref] %s", key, entry.Value)
	case model.ValueTypeString:
		return fmt.Sprintf("%s [string] %s", key, entry.Value)
	default:
		return fmt.Sprintf("%s [%s] not implemented", key, entry.ValueType)
	}
}

func preview(value string, max int) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func humanKB(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024.0)
}

func sortKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
