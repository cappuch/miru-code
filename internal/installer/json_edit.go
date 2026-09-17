package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/tailscale/hujson"
)

// Refuse ambiguous duplicate keys, rather than editing a different value from
// the one an agent will read.
func validateJSONObjects(v *hujson.Value) error {
	var invalid error
	v.Range(func(v *hujson.Value) bool {
		if o, ok := v.Value.(*hujson.Object); ok {
			seen := map[string]bool{}
			for _, m := range o.Members {
				name := m.Name.Value.(hujson.Literal).String()
				if seen[name] {
					invalid = fmt.Errorf("duplicate JSON object key; configuration left unchanged")
					return false
				}
				seen[name] = true
			}
		}
		return invalid == nil
	})
	return invalid
}

func objectMember(o *hujson.Object, key string) *hujson.Value {
	for i := range o.Members {
		if o.Members[i].Name.Value.(hujson.Literal).String() == key {
			return &o.Members[i].Value
		}
	}
	return nil
}

func appendMember(o *hujson.Object, key string, value hujson.Value) {
	name, _ := json.Marshal(key)
	o.Members = append(o.Members, hujson.ObjectMember{
		Name:  hujson.Value{BeforeExtra: hujson.Extra("\n  "), Value: hujson.Literal(name)},
		Value: value,
	})
}

func editJSONMember(path, sectionKey, memberKey string, value map[string]any, remove bool) (InstallAction, error) {
	// Follow existing symlinks, preserving the user's link itself.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return ActionError, err
	}
	original, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		return ActionError, fmt.Errorf("read %s: %w", path, err)
	}
	if !existed && remove {
		return ActionNotFound, nil
	}
	input := original
	if len(bytes.TrimSpace(input)) == 0 {
		input = []byte("{}\n")
	}
	doc, err := hujson.Parse(input)
	if err != nil {
		return ActionError, fmt.Errorf("parse %s (left unchanged): %w", path, err)
	}
	root, ok := doc.Value.(*hujson.Object)
	if !ok {
		return ActionError, fmt.Errorf("%s: expected a JSON object; left unchanged", path)
	}
	if err := validateJSONObjects(&doc); err != nil {
		return ActionError, fmt.Errorf("%s: %w", path, err)
	}
	section := objectMember(root, sectionKey)
	if section == nil {
		if remove {
			return ActionNotFound, nil
		}
		appendMember(root, sectionKey, hujson.Value{Value: &hujson.Object{}})
		section = objectMember(root, sectionKey)
	}
	o, ok := section.Value.(*hujson.Object)
	if !ok {
		return ActionError, fmt.Errorf("%s: %s must be an object; left unchanged", path, sectionKey)
	}
	member := objectMember(o, memberKey)
	if remove {
		if member == nil {
			return ActionNotFound, nil
		}
		escape := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
		patch, _ := json.Marshal([]map[string]string{{"op": "remove", "path": "/" + escape(sectionKey) + "/" + escape(memberKey)}})
		if err := doc.Patch(patch); err != nil {
			return ActionError, err
		}
	} else {
		encoded, err := json.MarshalIndent(value, "  ", "  ")
		if err != nil {
			return ActionError, err
		}
		if member != nil {
			old := member.Clone()
			old.Standardize()
			var a, b any
			if json.Unmarshal(old.Pack(), &a) == nil && json.Unmarshal(encoded, &b) == nil && reflect.DeepEqual(a, b) {
				return ActionUnchanged, nil
			}
		}
		replacement, err := hujson.Parse(encoded)
		if err != nil {
			return ActionError, err
		}
		if member != nil {
			member.Value = replacement.Value
		} else {
			appendMember(o, memberKey, replacement)
		}
	}
	if err := saveJSONConfig(path, original, doc.Pack(), existed); err != nil {
		return ActionError, err
	}
	if remove {
		return ActionRemoved, nil
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}

func saveJSONConfig(path string, original, next []byte, existed bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if existed {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
		backup, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".miru-backup-*")
		if err != nil {
			return err
		}
		_, writeErr := backup.Write(original)
		closeErr := backup.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".miru-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(next)
	if writeErr == nil {
		writeErr = f.Chmod(mode)
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.ReadFile(path)
	if (existed && (err != nil || !bytes.Equal(current, original))) || (!existed && !os.IsNotExist(err)) {
		return fmt.Errorf("%s changed during installation; refusing to overwrite it", path)
	}
	return os.Rename(f.Name(), path)
}
