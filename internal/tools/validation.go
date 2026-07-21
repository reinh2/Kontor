package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type compiledDefinition struct {
	definition Definition
	schema     *jsonschema.Schema
}

func compileDefinitions() (map[string]compiledDefinition, error) {
	compiled := make(map[string]compiledDefinition, len(definitions))
	for i, definition := range definitions {
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(definition.Parameters))
		if err != nil {
			return nil, fmt.Errorf("decode %s schema: %w", definition.Name, err)
		}
		location := fmt.Sprintf("https://kontor.invalid/schemas/tools/%s/v1.json", definition.Name)
		if err := compiler.AddResource(location, document); err != nil {
			return nil, fmt.Errorf("add %s schema: %w", definition.Name, err)
		}
		sch, err := compiler.Compile(location)
		if err != nil {
			return nil, fmt.Errorf("compile %s schema: %w", definition.Name, err)
		}
		if _, exists := compiled[definition.Name]; exists {
			return nil, fmt.Errorf("duplicate tool definition at index %d: %s", i, definition.Name)
		}
		compiled[definition.Name] = compiledDefinition{definition: definition, schema: sch}
	}
	return compiled, nil
}

func validateArguments(schema *jsonschema.Schema, raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return nil, err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("arguments must be one valid JSON object")
	}
	object, ok := instance.(map[string]any)
	if !ok {
		return nil, errors.New("arguments must be a JSON object")
	}
	if path, forbidden := findForbiddenIdentityField(object, ""); forbidden {
		return nil, fmt.Errorf("trusted identity field is forbidden at %s", path)
	}
	if err := schema.Validate(instance); err != nil {
		return nil, errors.New("arguments do not match the tool's v1 JSON Schema")
	}
	return object, nil
}

var forbiddenIdentityFields = map[string]struct{}{
	"tenant_id":           {},
	"customer_id":         {},
	"principal_id":        {},
	"owner_id":            {},
	"owner_customer_id":   {},
	"conversation_id":     {},
	"role":                {},
	"capability":          {},
	"capabilities":        {},
	"bypass_confirmation": {},
	"is_admin":            {},
}

func findForbiddenIdentityField(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			if _, forbidden := forbiddenIdentityFields[strings.ToLower(key)]; forbidden {
				return childPath, true
			}
			if found, forbidden := findForbiddenIdentityField(child, childPath); forbidden {
				return found, true
			}
		}
	case []any:
		for i, child := range typed {
			if found, forbidden := findForbiddenIdentityField(child, fmt.Sprintf("%s/%d", path, i)); forbidden {
				return found, true
			}
		}
	}
	return "", false
}

func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("arguments contain more than one JSON value")
		}
		return errors.New("arguments are not valid JSON")
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return errors.New("arguments are not valid JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return errors.New("arguments are not valid JSON")
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("arguments object contains a non-string key")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("arguments contain duplicate key %q", key)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("arguments are not valid JSON")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("arguments are not valid JSON")
		}
	default:
		return errors.New("arguments are not valid JSON")
	}
	return nil
}
