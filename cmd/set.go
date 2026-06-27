package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"ctx/internal/model"
)

func Set(ctx context.Context, args []string) error {
	parsed, err := parseSetArgs(args)
	if err != nil {
		return err
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	if parsed.valueType == model.ValueTypeString {
		return a.SetValue(ctx, parsed.sessionID, parsed.key, parsed.value)
	}
	return a.SetEntry(ctx, parsed.sessionID, parsed.key, model.NewEntry(parsed.value, parsed.valueType))
}

type setArgs struct {
	sessionID string
	key       string
	value     string
	valueType model.ValueType
}

func parseSetArgs(args []string) (setArgs, error) {
	if len(args) < 2 {
		return setArgs{}, usage("set", "ctx set [session_id] <key> <value>|--doc [text]|--path <path>")
	}
	flagIndex := -1
	for i, arg := range args {
		if arg == "--doc" || arg == "--path" {
			if flagIndex != -1 {
				return setArgs{}, fmt.Errorf("--doc and --path are mutually exclusive")
			}
			flagIndex = i
		}
	}
	if flagIndex == -1 {
		if len(args) != 2 && len(args) != 3 {
			return setArgs{}, usage("set", "ctx set [session_id] <key> <value>")
		}
		hasSession := len(args) == 3
		sessionID, err := sessionArg(args, hasSession)
		if err != nil {
			return setArgs{}, err
		}
		offset := 0
		if hasSession {
			offset = 1
		}
		return setArgs{sessionID: sessionID, key: args[offset], value: args[offset+1], valueType: model.ValueTypeString}, nil
	}

	if flagIndex != 1 && flagIndex != 2 {
		return setArgs{}, usage("set", "ctx set [session_id] <key> --doc [text] | ctx set [session_id] <key> --path <path>")
	}
	hasSession := flagIndex == 2
	sessionID, err := sessionArg(args, hasSession)
	if err != nil {
		return setArgs{}, err
	}
	offset := 0
	if hasSession {
		offset = 1
	}
	key := args[offset]
	flag := args[flagIndex]
	values := args[flagIndex+1:]

	switch flag {
	case "--doc":
		if len(values) > 1 {
			return setArgs{}, fmt.Errorf("--doc accepts at most one content argument")
		}
		value := ""
		if len(values) == 1 {
			value = values[0]
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return setArgs{}, fmt.Errorf("read doc from stdin: %w", err)
			}
			value = string(data)
		}
		return setArgs{sessionID: sessionID, key: key, value: value, valueType: model.ValueTypeDoc}, nil
	case "--path":
		if len(values) != 1 {
			return setArgs{}, fmt.Errorf("--path requires exactly one path")
		}
		return setArgs{sessionID: sessionID, key: key, value: values[0], valueType: model.ValueTypeFileRef}, nil
	default:
		return setArgs{}, usage("set", "ctx set [session_id] <key> <value>|--doc [text]|--path <path>")
	}
}
