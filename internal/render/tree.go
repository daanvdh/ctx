package render

import (
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
			Parent: node.Parent,
			Data:   node.Data,
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
		sb.WriteString(indent + k + "=" + s.Data[k] + "\n")
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

func sortKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
